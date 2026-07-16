package aiknowledge

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestReplaceChunksWritesAtMostFiveHundredRowsPerInsert(t *testing.T) {
	repo, mock, closeDB := newKnowledgeRepositoryMock(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec("UPDATE `ai_knowledge_chunks`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO `ai_knowledge_chunks`").WillReturnResult(sqlmock.NewResult(1, 500))
	mock.ExpectExec("INSERT INTO `ai_knowledge_chunks`").WillReturnResult(sqlmock.NewResult(501, 1))
	mock.ExpectExec("UPDATE `ai_knowledge_documents`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	chunks := make([]TextChunk, 501)
	for i := range chunks {
		chunks[i] = TextChunk{Index: uint(i + 1), Content: "chunk", Chars: 5}
	}
	err := repo.ReplaceChunks(context.Background(), KnowledgeDocument{ID: 7, KnowledgeBaseID: 3, Title: "doc"}, chunks, time.Date(2026, 7, 16, 1, 2, 3, 0, time.UTC))
	if err != nil {
		t.Fatalf("ReplaceChunks returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("chunk writes were not bounded: %v", err)
	}
}

func TestInsertRetrievalHitsUsesTransactionAndFiveHundredRowBatches(t *testing.T) {
	repo, mock, closeDB := newKnowledgeRepositoryMock(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `ai_knowledge_retrieval_hits`").WillReturnResult(sqlmock.NewResult(1, 500))
	mock.ExpectExec("INSERT INTO `ai_knowledge_retrieval_hits`").WillReturnResult(sqlmock.NewResult(501, 1))
	mock.ExpectCommit()

	hits := make([]ScoredHit, 501)
	for i := range hits {
		hits[i] = ScoredHit{KnowledgeBaseID: 3, DocumentID: 7, ChunkID: uint64(i + 1), RankNo: uint(i + 1), Content: "hit"}
	}
	if err := repo.InsertRetrievalHits(context.Background(), 11, hits); err != nil {
		t.Fatalf("InsertRetrievalHits returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("hit writes were not transactional and bounded: %v", err)
	}
}

func newKnowledgeRepositoryMock(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("gorm.Open: %v", err)
	}
	return &GormRepository{db: db}, mock, func() { _ = sqlDB.Close() }
}
