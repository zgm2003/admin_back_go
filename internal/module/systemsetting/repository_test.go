package systemsetting

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"admin_back_go/internal/shared/enum"

	"github.com/DATA-DOG/go-sqlmock"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type fakeSettingCache struct {
	payload    string
	getErr     error
	setErr     error
	deleteErr  error
	setKey     string
	setPayload string
	setTTL     time.Duration
	deletedKey string
}

func (f *fakeSettingCache) Get(context.Context, string) (string, error) {
	return f.payload, f.getErr
}

func (f *fakeSettingCache) Set(_ context.Context, key string, payload string, ttl time.Duration) error {
	f.setKey, f.setPayload, f.setTTL = key, payload, ttl
	return f.setErr
}

func (f *fakeSettingCache) Delete(_ context.Context, key string) error {
	f.deletedKey = key
	return f.deleteErr
}

func newRepositoryTestDatabase(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
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
		t.Fatalf("open gorm: %v", err)
	}
	return db, mock
}

func TestSettingByKeyReturnsMatchingCacheEntry(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	payload, err := json.Marshal(Setting{
		ID: 15, SettingKey: "auth.captcha.ttl_minutes", SettingValue: "2",
		ValueType: enum.SystemSettingValueNumber, Status: enum.CommonYes, IsDel: enum.CommonNo,
	})
	if err != nil {
		t.Fatalf("marshal cache fixture: %v", err)
	}
	repository := &GormRepository{db: db, cache: &fakeSettingCache{payload: string(payload)}}

	row, err := repository.SettingByKey(context.Background(), "auth.captcha.ttl_minutes")
	if err != nil || row == nil || row.ID != 15 || row.SettingValue != "2" {
		t.Fatalf("SettingByKey() row=%#v err=%v", row, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("cache hit queried MySQL: %v", err)
	}
}

func TestSettingByKeyCachesMySQLResult(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	cache := &fakeSettingCache{getErr: redis.Nil}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `system_settings` WHERE setting_key = ? AND is_del = ? ORDER BY `system_settings`.`id` LIMIT ?")).
		WithArgs("upload.token.ttl_minutes", enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "setting_key", "setting_value", "value_type", "remark", "status", "is_del", "created_at", "updated_at",
		}).AddRow(19, "upload.token.ttl_minutes", "15", enum.SystemSettingValueNumber, "上传临时凭证有效期分钟数", enum.CommonYes, enum.CommonNo, time.Now(), time.Now()))
	repository := &GormRepository{db: db, cache: cache}

	row, err := repository.SettingByKey(context.Background(), "upload.token.ttl_minutes")
	if err != nil || row == nil || row.ID != 19 {
		t.Fatalf("SettingByKey() row=%#v err=%v", row, err)
	}
	if cache.setKey != "sys_setting_raw_upload_token_ttl_minutes" || cache.setTTL != 5*time.Minute {
		t.Fatalf("cache write key=%q ttl=%s", cache.setKey, cache.setTTL)
	}
	var cached Setting
	if err := json.Unmarshal([]byte(cache.setPayload), &cached); err != nil || cached.ID != 19 {
		t.Fatalf("cached payload=%q row=%#v err=%v", cache.setPayload, cached, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestSettingByKeyFallsBackWhenCacheFails(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	cache := &fakeSettingCache{getErr: errors.New("redis unavailable"), setErr: errors.New("redis unavailable")}
	mock.ExpectQuery("SELECT .* FROM .*system_settings.*setting_key.*is_del.*LIMIT").
		WithArgs("auth.captcha.ttl_minutes", enum.CommonNo, 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "setting_key", "setting_value", "value_type", "remark", "status", "is_del", "created_at", "updated_at",
		}).AddRow(15, "auth.captcha.ttl_minutes", "2", enum.SystemSettingValueNumber, "", enum.CommonYes, enum.CommonNo, time.Now(), time.Now()))
	repository := &GormRepository{db: db, cache: cache}

	row, err := repository.SettingByKey(context.Background(), "auth.captcha.ttl_minutes")
	if err != nil || row == nil || row.ID != 15 {
		t.Fatalf("SettingByKey() row=%#v err=%v", row, err)
	}
	cache.deleteErr = errors.New("redis unavailable")
	repository.InvalidateCache(context.Background(), row.SettingKey)
	if cache.deletedKey != "sys_setting_raw_auth_captcha_ttl_minutes" {
		t.Fatalf("deleted key=%q", cache.deletedKey)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestFindByKeyIncludesSoftDeletedSetting(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	mock.ExpectQuery("SELECT .* FROM .*system_settings.*setting_key.*ORDER BY .* LIMIT").
		WithArgs("test", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "setting_key", "setting_value", "value_type", "remark", "status", "is_del", "created_at", "updated_at",
		}).AddRow(20, "test", "old", enum.SystemSettingValueString, "", enum.CommonYes, enum.CommonYes, time.Now(), time.Now()))

	repository := &GormRepository{db: db}
	row, err := repository.FindByKey(context.Background(), " test ")
	if err != nil || row == nil || row.ID != 20 || row.IsDel != enum.CommonYes {
		t.Fatalf("FindByKey() row=%#v err=%v", row, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestRestoreUpdatesOnlySoftDeletedSetting(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	mock.ExpectExec("UPDATE .*system_settings.*WHERE id = \\? AND is_del = \\?").
		WillReturnResult(sqlmock.NewResult(0, 1))

	repository := &GormRepository{db: db}
	restored, err := repository.Restore(context.Background(), 20, Setting{
		SettingValue: "42", ValueType: enum.SystemSettingValueNumber, Remark: "restored",
	})
	if err != nil || !restored {
		t.Fatalf("Restore() restored=%v err=%v", restored, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func TestCreateMapsMySQLDuplicateKey(t *testing.T) {
	db, mock := newRepositoryTestDatabase(t)
	mock.ExpectExec("INSERT INTO .*system_settings").
		WillReturnError(&mysqlDriver.MySQLError{Number: 1062, Message: "Duplicate entry 'test' for key 'uniq_setting_key'"})

	repository := &GormRepository{db: db}
	id, err := repository.Create(context.Background(), Setting{
		SettingKey: "test", SettingValue: "42", ValueType: enum.SystemSettingValueNumber,
		Status: enum.CommonYes, IsDel: enum.CommonNo,
	})
	if id != 0 || !errors.Is(err, ErrDuplicateKey) {
		t.Fatalf("Create() id=%d err=%v, want ErrDuplicateKey", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
