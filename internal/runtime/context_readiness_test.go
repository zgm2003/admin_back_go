package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"admin_back_go/internal/infra/contextindex"
	"admin_back_go/internal/readiness"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type fakeContextIndex struct {
	err      error
	profiles []contextindex.ActiveCollection
}

func (index *fakeContextIndex) CheckReadiness(_ context.Context, profiles []contextindex.ActiveCollection) error {
	index.profiles = append([]contextindex.ActiveCollection(nil), profiles...)
	return index.err
}

type fakeContextSources struct {
	profiles []contextindex.ActiveCollection
	err      error
}

func (sources fakeContextSources) ActiveCollections(context.Context) ([]contextindex.ActiveCollection, error) {
	return append([]contextindex.ActiveCollection(nil), sources.profiles...), sources.err
}

func TestAPIContextReadinessDegradesWhenQdrantIsUnavailable(t *testing.T) {
	index := &fakeContextIndex{err: errors.New("dial qdrant://secret@internal:6334: down")}
	checker := NewContextReadiness(index, fakeContextSources{})
	if got := checker.Check(t.Context()); got.Status != readiness.StatusDegraded {
		t.Fatalf("pure chat should expose degraded qdrant, got %#v", got)
	} else if strings.Contains(got.Message, "secret") || strings.Contains(got.Message, "6334") {
		t.Fatalf("readiness leaked qdrant credentials or address: %#v", got)
	}

	active := contextindex.ActiveCollection{ProfileID: 7, IndexGeneration: 3, DenseDimensions: 1536, DenseDistance: contextindex.DistanceCosine}
	index = &fakeContextIndex{err: errors.New("down")}
	checker = NewContextReadiness(index, fakeContextSources{profiles: []contextindex.ActiveCollection{active}})
	if got := checker.Check(t.Context()); got.Status != readiness.StatusDegraded {
		t.Fatalf("API availability must not depend on active context sources, got %#v", got)
	}
	if len(index.profiles) != 1 || index.profiles[0] != active {
		t.Fatalf("active collection contract was not checked: %#v", index.profiles)
	}
}

func TestContextReadinessFailsClosedWhenSourceStateCannotBeRead(t *testing.T) {
	checker := NewContextReadiness(&fakeContextIndex{}, fakeContextSources{err: errors.New("mysql query failed")})
	if got := checker.Check(t.Context()); got.Status != readiness.StatusDown {
		t.Fatalf("unknown source state must fail closed, got %#v", got)
	}
}

func TestWorkerContextReadinessAlwaysRequiresQdrant(t *testing.T) {
	checker := NewWorkerContextReadiness(&fakeContextIndex{err: errors.New("down")}, fakeContextSources{})
	if got := checker.Check(t.Context()); got.Status != readiness.StatusDown {
		t.Fatalf("worker must require qdrant without active sources, got %#v", got)
	}
}

func TestContextReadinessSourcesSelectEnabledDocumentsOrConversationTurns(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("(?s)SELECT DISTINCT.*FROM ai_context_profiles AS p.*ai_context_document_versions AS v.*OR EXISTS.*ai_conversations AS c.*ORDER BY p.id ASC").
		WillReturnRows(sqlmock.NewRows([]string{"profile_id", "index_generation", "index_state", "dense_dimensions", "dense_distance"}).
			AddRow(uint64(7), uint64(3), "ready", uint64(1536), "cosine"))

	collections, err := newGormContextSources(db).ActiveCollections(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := contextindex.ActiveCollection{ProfileID: 7, IndexGeneration: 3, DenseDimensions: 1536, DenseDistance: contextindex.DistanceCosine}
	if len(collections) != 1 || collections[0] != want {
		t.Fatalf("collections=%#v want=%#v", collections, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestContextReadinessFailsOnInconsistentActiveSourceProfile(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("(?s)SELECT DISTINCT.*FROM ai_context_profiles AS p.*ai_context_document_versions AS v.*OR EXISTS.*ai_conversations AS c.*ORDER BY p.id ASC").
		WillReturnRows(sqlmock.NewRows([]string{"profile_id", "index_generation", "index_state", "dense_dimensions", "dense_distance"}).
			AddRow(uint64(7), uint64(3), "failed", uint64(1536), "cosine"))
	if _, err := newGormContextSources(db).ActiveCollections(t.Context()); err == nil {
		t.Fatal("failed profile with an active source was reported ready")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
