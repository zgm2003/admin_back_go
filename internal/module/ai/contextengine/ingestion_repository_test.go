package contextengine

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestLoadWorkUsesSourceRunPlatformForConversationDocument(t *testing.T) {
	repository, mock, closeDB := newIngestionRepositoryFixture(t)
	defer closeDB()

	const (
		versionID      = uint64(19)
		documentID     = uint64(23)
		profileID      = uint64(2)
		conversationID = uint64(41)
		messageID      = uint64(105)
	)
	attachment := `{"attachments":[{"type":"file","object_key":"ai_chat_attachments/notes.txt","mime_type":"text/plain","name":"notes.txt","size":12,"etag":"etag-1"}]}`

	mock.ExpectQuery("(?s)SELECT .* FROM `ai_context_document_versions` WHERE id = \\?.*LIMIT \\?").
		WillReturnRows(ingestionVersionRows().AddRow(versionID, documentID, profileID, "cos", "ai_chat_attachments/notes.txt", "etag-1", int64(12), "text/plain", "notes.txt", DocumentVersionProcessing, uint32(1), nil, nil))
	mock.ExpectQuery("(?s)SELECT .* FROM `ai_context_profiles` WHERE id = \\?.*LIMIT \\?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "active_index_generation"}).AddRow(profileID, uint64(3)))
	mock.ExpectQuery("(?s)SELECT .* FROM `ai_context_documents` WHERE .*id = \\?.*deleted_at IS NULL.*LIMIT \\?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "conversation_id", "source_message_id", "source_attachment_index", "status", "deleted_at"}).
			AddRow(documentID, conversationID, messageID, uint32(0), DocumentEnabled, nil))
	mock.ExpectQuery("(?s)SELECT .*d\\.conversation_id.*FROM ai_context_document_versions AS v.*WHERE v\\.id = \\?.*LIMIT \\?").
		WillReturnRows(sqlmock.NewRows([]string{
			"conversation_id", "source_message_id", "source_attachment_index", "document_status", "active_version_id", "profile_id",
			"source_storage_provider", "source_object_key", "source_etag", "source_size_bytes", "source_mime_type", "source_filename",
			"agent_profile_id", "meta_json",
		}).AddRow(conversationID, messageID, uint32(0), DocumentEnabled, nil, profileID,
			"cos", "ai_chat_attachments/notes.txt", "etag-1", int64(12), "text/plain", "notes.txt", profileID, attachment))
	mock.ExpectQuery("(?s)SELECT .*platform.*user_id.*FROM ai_runs.*conversation_id = \\?.*user_message_id = \\?.*LIMIT \\?").
		WillReturnRows(sqlmock.NewRows([]string{"platform", "user_id"}).AddRow("admin", uint64(7)))

	work, err := repository.loadWork(context.Background(), versionID)
	if err != nil {
		t.Fatal(err)
	}
	if work.Platform != "admin" || work.UserID != 7 || work.ConversationID != conversationID {
		t.Fatalf("work platform=%q user_id=%d conversation_id=%d", work.Platform, work.UserID, work.ConversationID)
	}
	assertIngestionMockExpectations(t, mock)
}

func TestAcquireVersionLeaseRollsBackWhenWorkCannotLoad(t *testing.T) {
	repository, mock, closeDB := newIngestionRepositoryFixture(t)
	defer closeDB()

	now := time.Date(2026, time.August, 7, 10, 0, 0, 0, time.UTC)
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)SELECT .* FROM `ai_context_document_versions` WHERE id = \\?.*LIMIT \\? FOR UPDATE").
		WillReturnRows(ingestionVersionRows().AddRow(uint64(19), uint64(23), uint64(2), "cos", "ai_chat_attachments/notes.txt", "etag-1", int64(12), "text/plain", "notes.txt", DocumentVersionQueued, uint32(0), nil, nil))
	mock.ExpectExec("(?s)UPDATE `ai_context_document_versions` SET .*attempt_count.*lease_expires_at.*lease_token.*started_at.*state.*WHERE id = \\?.*state IN \\(\\?,\\?\\)").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("(?s)SELECT .* FROM `ai_context_document_versions` WHERE id = \\?.*LIMIT \\?").
		WillReturnRows(ingestionVersionRows().AddRow(uint64(19), uint64(23), uint64(2), "cos", "ai_chat_attachments/notes.txt", "etag-1", int64(12), "text/plain", "notes.txt", DocumentVersionProcessing, uint32(1), uint64(9), now.Add(time.Minute)))
	mock.ExpectQuery("(?s)SELECT .* FROM `ai_context_profiles` WHERE id = \\?.*LIMIT \\?").
		WillReturnError(errors.New("profile load failed"))
	mock.ExpectRollback()

	_, _, err := repository.AcquireVersionLease(context.Background(), 19, now, time.Minute)
	if err == nil || !strings.Contains(err.Error(), "profile load failed") {
		t.Fatalf("AcquireVersionLease error=%v, want profile load failure", err)
	}
	assertIngestionMockExpectations(t, mock)
}

func newIngestionRepositoryFixture(t *testing.T) (*GormIngestionRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(
		sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp),
		sqlmock.ValueConverterOption(unsignedSQLMockConverter{}),
	)
	if err != nil {
		t.Fatal(err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return &GormIngestionRepository{db: db}, mock, func() { _ = sqlDB.Close() }
}

type unsignedSQLMockConverter struct{}

func (unsignedSQLMockConverter) ConvertValue(value any) (driver.Value, error) {
	if number, ok := value.(uint64); ok {
		return strconv.FormatUint(number, 10), nil
	}
	return driver.DefaultParameterConverter.ConvertValue(value)
}

func ingestionVersionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "document_id", "profile_id", "source_storage_provider", "source_object_key", "source_etag", "source_size_bytes",
		"source_mime_type", "source_filename", "state", "attempt_count", "lease_token", "lease_expires_at",
	})
}

func assertIngestionMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(regexp.MustCompile(`\\s+`).ReplaceAllString(err.Error(), " "))
	}
}
