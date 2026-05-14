package mail

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/dict"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/platform/database"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMailDictsReturnRequiredValues(t *testing.T) {
	if got := len(dict.MailSceneOptions()); got != 4 {
		t.Fatalf("mail scene dict count = %d, want 4", got)
	}
	if got := len(dict.MailLogSceneOptions()); got != 5 {
		t.Fatalf("mail log scene dict count = %d, want 5", got)
	}
	if got := len(dict.MailLogStatusOptions()); got != 3 {
		t.Fatalf("mail log status dict count = %d, want 3", got)
	}
}

func TestMailModelsKeepSoftDeleteFields(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Config{}),
		reflect.TypeOf(Template{}),
		reflect.TypeOf(Log{}),
	} {
		field, ok := typ.FieldByName("IsDel")
		if !ok {
			t.Fatalf("%s must expose IsDel", typ.Name())
		}
		if field.Type.Kind() != reflect.Int {
			t.Fatalf("%s.IsDel must be int, got %s", typ.Name(), field.Type)
		}
		if tag := field.Tag.Get("gorm"); !strings.Contains(tag, "column:is_del") {
			t.Fatalf("%s.IsDel must map to is_del, got tag %q", typ.Name(), tag)
		}
	}
}

func TestRepositoryReadContractsRequireIsDelFilter(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectQuery("SELECT \\* FROM `mail_configs` WHERE config_key = \\? AND is_del = \\? ORDER BY `mail_configs`.`id` LIMIT \\?").
		WithArgs(defaultConfigKey, enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `mail_templates` WHERE is_del = \\? ORDER BY id DESC").
		WithArgs(enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `mail_templates` WHERE id = \\? AND is_del = \\? ORDER BY `mail_templates`.`id` LIMIT \\?").
		WithArgs(uint64(11), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `mail_templates` WHERE scene = \\? AND is_del = \\? ORDER BY `mail_templates`.`id` LIMIT \\?").
		WithArgs(enum.VerifyCodeSceneLogin, enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `mail_logs` WHERE is_del = \\?").
		WithArgs(enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery("SELECT \\* FROM `mail_logs` WHERE is_del = \\? ORDER BY created_at DESC, id DESC LIMIT \\?").
		WithArgs(enum.CommonNo, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery("SELECT \\* FROM `mail_logs` WHERE id = \\? AND is_del = \\? ORDER BY `mail_logs`.`id` LIMIT \\?").
		WithArgs(uint64(12), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	if _, err := repo.DefaultConfig(context.Background()); err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	if _, err := repo.ListTemplates(context.Background()); err != nil {
		t.Fatalf("ListTemplates returned error: %v", err)
	}
	if _, err := repo.TemplateByID(context.Background(), 11); err != nil {
		t.Fatalf("TemplateByID returned error: %v", err)
	}
	if _, err := repo.TemplateByScene(context.Background(), enum.VerifyCodeSceneLogin); err != nil {
		t.Fatalf("TemplateByScene returned error: %v", err)
	}
	if _, _, err := repo.ListLogs(context.Background(), LogQuery{CurrentPage: 1, PageSize: 20}); err != nil {
		t.Fatalf("ListLogs returned error: %v", err)
	}
	if _, err := repo.LogByID(context.Background(), 12); err != nil {
		t.Fatalf("LogByID returned error: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositorySaveDefaultConfigRestoresSoftDeletedDefault(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `mail_configs` WHERE config_key = ? ORDER BY `mail_configs`.`id` LIMIT ? FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "config_key", "secret_id_enc", "secret_id_hint", "secret_key_enc", "secret_key_hint", "region", "endpoint", "from_email", "from_name", "reply_to", "status", "is_del", "created_at", "updated_at"}).
			AddRow(uint64(7), defaultConfigKey, "old-id", "***d-id", "old-key", "***-key", DefaultRegion, DefaultEndpoint, "old@example.com", "old", "", enum.CommonNo, enum.CommonYes, time.Now(), time.Now()))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `mail_configs` SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.SaveDefaultConfig(context.Background(), Config{
		SecretIDEnc: "new-id", SecretIDHint: "***w-id", SecretKeyEnc: "new-key", SecretKeyHint: "***-key",
		Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", Status: enum.CommonYes,
	})
	if err != nil {
		t.Fatalf("SaveDefaultConfig returned error: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositorySaveTemplateRestoresSoftDeletedScene(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `mail_templates` WHERE scene = ? ORDER BY `mail_templates`.`id` LIMIT ? FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scene", "name", "subject", "tencent_template_id", "variables_json", "sample_variables_json", "status", "is_del", "created_at", "updated_at"}).
			AddRow(uint64(9), enum.VerifyCodeSceneLogin, "old", "old", uint64(100), `["code"]`, `{"code":"123456"}`, enum.CommonNo, enum.CommonYes, time.Now(), time.Now()))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `mail_templates` SET")).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	id, err := repo.SaveTemplate(context.Background(), Template{
		Scene: enum.VerifyCodeSceneLogin, Name: "login", Subject: "Login code", TencentTemplateID: 200,
		VariablesJSON: `["code","ttl_minutes"]`, SampleVariablesJSON: `{"code":"123456","ttl_minutes":"5"}`, Status: enum.CommonYes,
	})
	if err != nil {
		t.Fatalf("SaveTemplate returned error: %v", err)
	}
	if id != 9 {
		t.Fatalf("expected restored template id 9, got %d", id)
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryListLogsFiltersSoftDeletedRows(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `mail_logs` WHERE is_del = \\?").
		WithArgs(enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))
	mock.ExpectQuery("SELECT \\* FROM `mail_logs` WHERE is_del = \\? ORDER BY created_at DESC, id DESC LIMIT \\?").
		WithArgs(enum.CommonNo, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scene", "template_id", "to_email", "subject", "tencent_request_id", "tencent_message_id", "status", "is_del", "error_code", "error_message", "duration_ms", "sent_at", "created_at", "updated_at"}).
			AddRow(uint64(1), enum.VerifyCodeSceneLogin, uint64(9), "user@example.com", "Login", "req", "msg", enum.MailLogStatusSuccess, enum.CommonNo, "", "", uint64(25), time.Now(), time.Now(), time.Now()))

	rows, total, err := repo.ListLogs(context.Background(), LogQuery{CurrentPage: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListLogs returned error: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].IsDel != enum.CommonNo {
		t.Fatalf("unexpected active logs: total=%d rows=%#v", total, rows)
	}
	assertMockExpectations(t, mock)
}

func newMockRepository(t *testing.T) (*GormRepository, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("create sql mock: %v", err)
	}
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatalf("open gorm mock db: %v", err)
	}
	client := &database.Client{Gorm: db, SQL: sqlDB}
	return NewGormRepository(client), mock, func() { _ = sqlDB.Close() }
}

func assertMockExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
