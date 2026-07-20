package permission

import (
	"context"
	"reflect"
	"testing"

	"admin_back_go/internal/infra/database"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPrincipalSubjectsForPlatformsBuildsIndependentCartesianProduct(t *testing.T) {
	got := PrincipalSubjectsForPlatforms(
		[]int64{3, 1, 3, 0},
		[]string{"partner_portal", "admin", "partner_portal", ""},
	)
	want := []PrincipalSubject{
		{UserID: 1, Platform: "admin"},
		{UserID: 3, Platform: "admin"},
		{UserID: 1, Platform: "partner_portal"},
		{UserID: 3, Platform: "partner_portal"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("principal subjects mismatch\nwant=%#v\n got=%#v", want, got)
	}
}

func TestGroupPrincipalSubjectsByPlatformKeepsIndependentIDs(t *testing.T) {
	groups := groupPrincipalSubjectsByPlatform([]PrincipalSubject{
		{UserID: 3, Platform: "partner_portal"},
		{UserID: 2, Platform: "admin"},
		{UserID: 1, Platform: "partner_portal"},
		{UserID: 1, Platform: "partner_portal"},
	})
	want := []principalSubjectGroup{
		{Platform: "admin", UserIDs: []int64{2}},
		{Platform: "partner_portal", UserIDs: []int64{1, 3}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("principal groups mismatch\nwant=%#v\n got=%#v", want, groups)
	}
}

func TestPrincipalSubjectUserIDsDeduplicateAcrossPlatforms(t *testing.T) {
	got := principalSubjectUserIDs([]PrincipalSubject{
		{UserID: 3, Platform: "partner_portal"},
		{UserID: 2, Platform: "admin"},
		{UserID: 3, Platform: "admin"},
	})
	if !reflect.DeepEqual(got, []int64{2, 3}) {
		t.Fatalf("principal user IDs mismatch: %#v", got)
	}
}

func TestCurrentVersionsKeepsRepositoryQueriesIsolatedByPlatform(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open gorm mock db: %v", err)
	}

	query := `SELECT .* FROM users AS u .*authz_principal_versions.*u\.id IN.*ORDER BY u\.id ASC`
	mock.ExpectQuery(query).
		WithArgs("admin", "admin", int64(2)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id", "platform", "version"}).AddRow(2, 20, "admin", 4))
	mock.ExpectQuery(query).
		WithArgs("partner_portal", "partner_portal", int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "role_id", "platform", "version"}).AddRow(1, 10, "partner_portal", 7))

	repository := NewGormPrincipalRepository(&database.Client{Gorm: db, SQL: sqlDB})
	got, err := repository.CurrentVersions(context.Background(), []PrincipalSubject{
		{UserID: 1, Platform: "partner_portal"},
		{UserID: 2, Platform: "admin"},
	})
	if err != nil {
		t.Fatalf("CurrentVersions() error = %v", err)
	}
	want := []PrincipalVersion{
		{UserID: 2, RoleID: 20, Platform: "admin", Version: 4},
		{UserID: 1, RoleID: 10, Platform: "partner_portal", Version: 7},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("principal versions mismatch\nwant=%#v\n got=%#v", want, got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
