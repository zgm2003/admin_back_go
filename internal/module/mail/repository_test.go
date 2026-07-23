package mail

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/dict"
	"admin_back_go/internal/shared/enum"

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
	field, ok := reflect.TypeOf(Config{}).FieldByName("VerifyCodeTTLMinutes")
	if !ok {
		t.Fatal("Config must expose VerifyCodeTTLMinutes")
	}
	if tag := field.Tag.Get("gorm"); !strings.Contains(tag, "column:verify_code_ttl_minutes") {
		t.Fatalf("Config.VerifyCodeTTLMinutes must map to verify_code_ttl_minutes, got tag %q", tag)
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
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `mail_logs` WHERE mail_logs.is_del = ?")).
		WithArgs(enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT mail_logs.*, mvc.id AS verification_snapshot_id, mvc.key_id AS verification_key_id, mvc.code_enc AS verification_code_enc, mvc.expires_at AS verification_expires_at FROM `mail_logs` LEFT JOIN mail_log_verification_codes AS mvc ON mvc.mail_log_id = mail_logs.id WHERE mail_logs.is_del = ? ORDER BY mail_logs.created_at DESC, mail_logs.id DESC LIMIT ?")).
		WithArgs(enum.CommonNo, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT mail_logs.*, mvc.id AS verification_snapshot_id, mvc.key_id AS verification_key_id, mvc.code_enc AS verification_code_enc, mvc.expires_at AS verification_expires_at FROM `mail_logs` LEFT JOIN mail_log_verification_codes AS mvc ON mvc.mail_log_id = mail_logs.id WHERE mail_logs.id = ? AND mail_logs.is_del = ? LIMIT ?")).
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
	if _, _, err := repo.ListLogRows(context.Background(), LogQuery{CurrentPage: 1, PageSize: 20}); err != nil {
		t.Fatalf("ListLogRows returned error: %v", err)
	}
	if _, err := repo.LogRowByID(context.Background(), 12); err != nil {
		t.Fatalf("LogRowByID returned error: %v", err)
	}
	assertMockExpectations(t, mock)
}

func TestRepositorySaveDefaultConfigRestoresSoftDeletedDefault(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `mail_configs` WHERE config_key = ? ORDER BY `mail_configs`.`id` LIMIT ? FOR UPDATE")).
		WillReturnRows(sqlmock.NewRows([]string{"id", "config_key", "secret_id_enc", "secret_id_hint", "secret_key_enc", "secret_key_hint", "region", "endpoint", "from_email", "from_name", "reply_to", "verify_code_ttl_minutes", "status", "is_del", "created_at", "updated_at"}).
			AddRow(uint64(7), defaultConfigKey, "old-id", "***d-id", "old-key", "***-key", DefaultRegion, DefaultEndpoint, "old@example.com", "old", "", 5, enum.CommonNo, enum.CommonYes, time.Now(), time.Now()))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `mail_configs` SET") + ".*`verify_code_ttl_minutes`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repo.SaveDefaultConfig(context.Background(), Config{
		SecretIDEnc: "new-id", SecretIDHint: "***w-id", SecretKeyEnc: "new-key", SecretKeyHint: "***-key",
		Region: DefaultRegion, Endpoint: DefaultEndpoint, FromEmail: "noreply@example.com", VerifyCodeTTLMinutes: 9, Status: enum.CommonYes,
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

func TestRepositoryListLogRowsPreservesNullableVerificationTuple(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `mail_logs` WHERE mail_logs.is_del = ?")).
		WithArgs(enum.CommonNo).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT mail_logs.*, mvc.id AS verification_snapshot_id, mvc.key_id AS verification_key_id, mvc.code_enc AS verification_code_enc, mvc.expires_at AS verification_expires_at FROM `mail_logs` LEFT JOIN mail_log_verification_codes AS mvc ON mvc.mail_log_id = mail_logs.id WHERE mail_logs.is_del = ? ORDER BY mail_logs.created_at DESC, mail_logs.id DESC LIMIT ?")).
		WithArgs(enum.CommonNo, 20).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scene", "template_id", "to_email", "subject", "tencent_request_id", "tencent_message_id", "status", "is_del", "error_code", "error_message", "duration_ms", "sent_at", "created_at", "updated_at", "verification_snapshot_id", "verification_key_id", "verification_code_enc", "verification_expires_at"}).
			AddRow(uint64(1), enum.VerifyCodeSceneLogin, uint64(9), "one@example.com", "Login", "req", "msg", enum.MailLogStatusSuccess, enum.CommonNo, "", "", uint64(25), time.Now(), time.Now(), time.Now(), nil, nil, nil, nil).
			AddRow(uint64(2), enum.VerifyCodeSceneLogin, uint64(9), "two@example.com", "Login", "", "", enum.MailLogStatusPending, enum.CommonNo, "", "", uint64(0), nil, time.Now(), time.Now(), uint64(31), nil, "cipher", nil))

	rows, total, err := repo.ListLogRows(context.Background(), LogQuery{CurrentPage: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("ListLogRows returned error: %v", err)
	}
	if total != 2 || len(rows) != 2 || rows[0].IsDel != enum.CommonNo {
		t.Fatalf("unexpected active logs: total=%d rows=%#v", total, rows)
	}
	if rows[0].VerificationSnapshotID != nil || rows[0].VerificationKeyID != nil || rows[0].VerificationCodeEnc != nil || rows[0].VerificationExpiresAt != nil {
		t.Fatalf("historical parent without child must preserve an all-null tuple: %#v", rows[0])
	}
	if rows[1].VerificationSnapshotID == nil || *rows[1].VerificationSnapshotID != 31 || rows[1].VerificationKeyID != nil || rows[1].VerificationCodeEnc == nil || *rows[1].VerificationCodeEnc != "cipher" || rows[1].VerificationExpiresAt != nil {
		t.Fatalf("partial child join must not synthesize missing values: %#v", rows[1])
	}
	assertMockExpectations(t, mock)
}

func TestRepositoryLogRowByIDPreservesHistoricalNullSnapshot(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectQuery(regexp.QuoteMeta("SELECT mail_logs.*, mvc.id AS verification_snapshot_id, mvc.key_id AS verification_key_id, mvc.code_enc AS verification_code_enc, mvc.expires_at AS verification_expires_at FROM `mail_logs` LEFT JOIN mail_log_verification_codes AS mvc ON mvc.mail_log_id = mail_logs.id WHERE mail_logs.id = ? AND mail_logs.is_del = ? LIMIT ?")).
		WithArgs(uint64(12), enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "scene", "is_del", "verification_snapshot_id", "verification_key_id", "verification_code_enc", "verification_expires_at"}).
			AddRow(uint64(12), enum.MailSceneTest, enum.CommonNo, nil, nil, nil, nil))

	row, err := repo.LogRowByID(context.Background(), 12)
	if err != nil {
		t.Fatalf("LogRowByID returned error: %v", err)
	}
	if row == nil || row.ID != 12 {
		t.Fatalf("unexpected log row: %#v", row)
	}
	if row.VerificationSnapshotID != nil || row.VerificationKeyID != nil || row.VerificationCodeEnc != nil || row.VerificationExpiresAt != nil {
		t.Fatalf("historical parent without child must preserve nulls: %#v", row)
	}
	assertMockExpectations(t, mock)
}

func TestCreateVerificationLogCommitsParentAndChildAtomically(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()
	expiresAt := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `mail_logs`").WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectExec("INSERT INTO `mail_log_verification_codes`").
		WithArgs(uint64(41), "current", "ciphertext", expiresAt, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(51, 1))
	mock.ExpectCommit()

	id, err := repo.CreateVerificationLog(context.Background(), Log{Scene: enum.VerifyCodeSceneLogin}, VerificationCodeSnapshot{
		KeyID: "current", CodeEnc: "ciphertext", ExpiresAt: expiresAt,
	})
	if err != nil || id != 41 {
		t.Fatalf("CreateVerificationLog id=%d err=%v", id, err)
	}
	assertMockExpectations(t, mock)
}

func TestCreateVerificationLogRollsBackWhenParentInsertFails(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `mail_logs`").WillReturnError(errors.New("parent insert failed"))
	mock.ExpectRollback()

	id, err := repo.CreateVerificationLog(context.Background(), Log{}, VerificationCodeSnapshot{})
	if err == nil {
		t.Fatal("expected parent insert failure")
	}
	if id != 0 {
		t.Fatalf("rolled-back parent must not return an id, got %d", id)
	}
	assertMockExpectations(t, mock)
}

func TestCreateVerificationLogRollsBackWhenChildInsertFails(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `mail_logs`").WillReturnResult(sqlmock.NewResult(41, 1))
	mock.ExpectExec("INSERT INTO `mail_log_verification_codes`").WillReturnError(errors.New("child insert failed"))
	mock.ExpectRollback()

	id, err := repo.CreateVerificationLog(context.Background(), Log{}, VerificationCodeSnapshot{})
	if err == nil {
		t.Fatal("expected child insert failure")
	}
	if id != 0 {
		t.Fatalf("rolled-back child transaction must not return a parent id, got %d", id)
	}
	assertMockExpectations(t, mock)
}

func TestCreateLogOnlyInsertsParent(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectExec("INSERT INTO `mail_logs`").WillReturnResult(sqlmock.NewResult(61, 1))

	id, err := repo.CreateLog(context.Background(), Log{Scene: enum.MailSceneTest})
	if err != nil || id != 61 {
		t.Fatalf("CreateLog id=%d err=%v", id, err)
	}
	assertMockExpectations(t, mock)
}

func TestFinishLogDoesNotMutateSnapshot(t *testing.T) {
	repo, mock, closeDB := newMockRepository(t)
	defer closeDB()

	mock.ExpectExec("UPDATE `mail_logs` SET").WithArgs(
		uint64(8), "", "", sqlmock.AnyArg(), enum.MailLogStatusSuccess, "", "", sqlmock.AnyArg(), uint64(41), enum.CommonNo,
	).WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.FinishLog(context.Background(), 41, LogFinish{Status: enum.MailLogStatusSuccess, DurationMS: 8})
	if err != nil {
		t.Fatalf("FinishLog returned error: %v", err)
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
