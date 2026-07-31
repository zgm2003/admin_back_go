package uploadtoken

import (
	"context"
	"errors"
	"math"
	"reflect"
	"testing"

	"admin_back_go/internal/shared/enum"
	"admin_back_go/internal/shared/uploadpolicy"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type activeRuleRepository struct {
	config       *EnabledConfig
	lockedConfig *EnabledConfig
	err          error
	lockedErr    error
	calls        int
	lockedTx     *gorm.DB
}

func (repository *activeRuleRepository) GetEnabledConfig(context.Context) (*EnabledConfig, error) {
	repository.calls++
	return repository.config, repository.err
}

func (repository *activeRuleRepository) GetEnabledConfigForUpdate(_ context.Context, tx *gorm.DB) (*EnabledConfig, error) {
	repository.lockedTx = tx
	if repository.lockedConfig != nil || repository.lockedErr != nil {
		return repository.lockedConfig, repository.lockedErr
	}
	return repository.config, nil
}

func TestActiveRuleResolverNormalizesCurrentEnabledRule(t *testing.T) {
	repository := &activeRuleRepository{config: validActiveRuleConfig()}
	resolver := NewActiveRuleResolver(repository)

	got, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("ResolveActive returned error: %v", err)
	}
	if got.MaxFileBytes != 100<<20 || !reflect.DeepEqual(got.ImageExtensions, []string{"jpeg", "png"}) ||
		!reflect.DeepEqual(got.FileExtensions, []string{"pdf", "md", "go", "zip"}) || got.ConsistencyToken == (uploadpolicy.ConsistencyToken{}) {
		t.Fatalf("active rule=%#v", got)
	}
}

func TestActiveRuleGuardRejectsChangedSnapshotUsingCallerTransaction(t *testing.T) {
	repository := &activeRuleRepository{config: validActiveRuleConfig(), lockedConfig: activeRuleConfigWith(10, `["png"]`, `["pdf"]`)}
	resolver := NewActiveRuleResolver(repository)
	rule, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tx := &gorm.DB{}

	err = resolver.GuardActiveInTransaction(context.Background(), tx, rule.ConsistencyToken)

	if !errors.Is(err, uploadpolicy.ErrRuleSnapshotChanged) {
		t.Fatalf("changed upload rule guard error=%v", err)
	}
	if repository.lockedTx != tx {
		t.Fatal("upload rule guard did not use the caller transaction")
	}
}

func TestActiveRuleGuardAcceptsMatchingSnapshotUsingCallerTransaction(t *testing.T) {
	repository := &activeRuleRepository{config: validActiveRuleConfig()}
	resolver := NewActiveRuleResolver(repository)
	rule, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	tx := &gorm.DB{}

	if err := resolver.GuardActiveInTransaction(context.Background(), tx, rule.ConsistencyToken); err != nil {
		t.Fatalf("matching upload rule snapshot rejected: %v", err)
	}
	if repository.lockedTx != tx {
		t.Fatal("upload rule guard did not use the caller transaction")
	}
}

func TestGormRepositoryLocksEnabledRuleForConsistencyGuard(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	repository := &GormRepository{db: db}
	columns := []string{
		"setting_id", "driver_id", "rule_id", "driver", "secret_id_enc", "secret_key_enc", "bucket", "region", "appid", "endpoint", "bucket_domain", "role_arn",
		"max_size_mb", "image_exts", "file_exts",
	}
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM upload_setting AS s.*FOR UPDATE`).
		WithArgs(enum.CommonNo, enum.CommonNo, enum.CommonYes, enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(1, 2, 3, "cos", "sid", "skey", "bucket", "ap-test", "", "", "", "", 100, `["png"]`, `["pdf"]`))
	mock.ExpectCommit()

	err = db.Transaction(func(tx *gorm.DB) error {
		config, queryErr := repository.GetEnabledConfigForUpdate(context.Background(), tx)
		if queryErr != nil {
			return queryErr
		}
		if config == nil || config.SettingID != 1 || config.RuleID != 3 {
			t.Fatalf("locked config=%#v", config)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestActiveRuleResolverFailsClosed(t *testing.T) {
	type testCase struct {
		name       string
		repository Repository
	}
	tests := []testCase{
		{name: "repository missing", repository: nil},
		{name: "enabled config missing", repository: &activeRuleRepository{}},
		{name: "repository error", repository: &activeRuleRepository{err: errors.New("database unavailable")}},
		{name: "zero max size", repository: &activeRuleRepository{config: activeRuleConfigWith(0, `["png"]`, `["pdf"]`)}},
		{name: "negative max size", repository: &activeRuleRepository{config: activeRuleConfigWith(-1, `["png"]`, `["pdf"]`)}},
		{name: "malformed image extensions", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `{`, `["pdf"]`)}},
		{name: "malformed file extensions", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `["png"]`, `{`)}},
		{name: "unknown image extension", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `["png","exe"]`, `["pdf"]`)}},
		{name: "unknown file extension", repository: &activeRuleRepository{config: activeRuleConfigWith(1, `["png"]`, `["pdf","exe"]`)}},
	}
	overflowMB64 := int64(math.MaxInt64)/(1<<20) + 1
	if overflowMB := int(overflowMB64); int64(overflowMB) == overflowMB64 {
		tests = append(tests, testCase{name: "max size overflow", repository: &activeRuleRepository{config: activeRuleConfigWith(overflowMB, `["png"]`, `["pdf"]`)}})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolver := NewActiveRuleResolver(test.repository)
			if _, err := resolver.ResolveActive(context.Background()); err == nil {
				t.Fatal("ResolveActive unexpectedly succeeded")
			}
		})
	}
}

func TestActiveRuleResolverReadsRepositoryOnEveryResolution(t *testing.T) {
	repository := &activeRuleRepository{config: validActiveRuleConfig()}
	resolver := NewActiveRuleResolver(repository)

	first, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("first ResolveActive returned error: %v", err)
	}
	repository.config = activeRuleConfigWith(2, `["avif"]`, `["yaml"]`)
	second, err := resolver.ResolveActive(context.Background())
	if err != nil {
		t.Fatalf("second ResolveActive returned error: %v", err)
	}

	if repository.calls != 2 {
		t.Fatalf("GetEnabledConfig calls=%d want=2", repository.calls)
	}
	if first.MaxFileBytes != 100<<20 || second.MaxFileBytes != 2<<20 {
		t.Fatalf("rules did not reflect repository changes: first=%#v second=%#v", first, second)
	}
	if !reflect.DeepEqual(second.ImageExtensions, []string{"avif"}) || !reflect.DeepEqual(second.FileExtensions, []string{"yaml"}) {
		t.Fatalf("second active rule=%#v", second)
	}
}

func TestUploadPolicyResolverFuncDelegates(t *testing.T) {
	called := false
	resolver := uploadpolicy.ResolverFunc(func(context.Context) (uploadpolicy.Rule, error) {
		called = true
		return uploadpolicy.Rule{MaxFileBytes: 1}, nil
	})

	got, err := resolver.ResolveActive(context.Background())
	if err != nil || !called || got.MaxFileBytes != 1 {
		t.Fatalf("ResolverFunc result=%#v called=%t err=%v", got, called, err)
	}
}

func validActiveRuleConfig() *EnabledConfig {
	return activeRuleConfigWith(100, `["png","jpeg"]`, `["pdf","md","go","zip"]`)
}

func activeRuleConfigWith(maxSizeMB int, imageExts string, fileExts string) *EnabledConfig {
	return &EnabledConfig{
		SettingID: 1,
		MaxSizeMB: maxSizeMB,
		ImageExts: imageExts,
		FileExts:  fileExts,
	}
}
