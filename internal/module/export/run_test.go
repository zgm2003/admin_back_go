package exporttask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/shared/enum"

	mysqldriver "github.com/go-sql-driver/mysql"
)

type fakeDataProvider struct {
	input              BuildInput
	data               *FileData
	err                error
	blockUntilCanceled bool
}

func (f *fakeDataProvider) BuildExportData(ctx context.Context, input BuildInput) (*FileData, error) {
	f.input = input
	if f.blockUntilCanceled {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.data, f.err
}

func testRegistry(t *testing.T, provider Provider) *Registry {
	t.Helper()
	registry, err := NewRegistry(Definition{Kind: KindUserList, Title: "用户列表", Provider: provider})
	if err != nil {
		t.Fatalf("NewRegistry returned error: %v", err)
	}
	return registry
}

type fakeFileWriter struct {
	data FileData
	body []byte
	err  error
}

func (f *fakeFileWriter) Write(data FileData) ([]byte, error) {
	f.data = data
	return f.body, f.err
}

type fakeUploader struct {
	input  UploadInput
	result *UploadResult
	err    error
}

func (f *fakeUploader) Upload(ctx context.Context, input UploadInput) (*UploadResult, error) {
	f.input = input
	return f.result, f.err
}

type fakeNotifier struct {
	success      NotifyInput
	failed       NotifyInput
	successCalls int
}

func (f *fakeNotifier) NotifyExportSuccess(ctx context.Context, input NotifyInput) error {
	f.success = input
	f.successCalls++
	return nil
}

func (f *fakeNotifier) NotifyExportFailed(ctx context.Context, input NotifyInput) error {
	f.failed = input
	return nil
}

func TestRunGeneratesUploadsMarksSuccessAndNotifies(t *testing.T) {
	createdAt := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepository{getRow: &Task{ID: 7, UserID: 9, Platform: enum.PlatformAdmin, Kind: KindUserList, Title: "用户列表导出", Status: enum.ExportTaskStatusPending, IsDel: enum.CommonNo, CreatedAt: createdAt}}
	provider := &fakeDataProvider{data: &FileData{Prefix: "用户列表导出", Headers: []Column{{Key: "id", Title: "ID"}}, Rows: []map[string]string{{"id": "3"}}}}
	writer := &fakeFileWriter{body: []byte("xlsx")}
	uploader := &fakeUploader{result: &UploadResult{FileName: "u.xlsx", FileURL: "https://cos/u.xlsx", ObjectKey: "exports/user_list/20260507/u.xlsx", FileSize: 4, RowCount: 1}}
	notifier := &fakeNotifier{}

	err := NewService(repo, WithDefinitionRegistry(testRegistry(t, provider)), WithFileWriter(writer), WithFileUploader(uploader), WithNotifier(notifier)).Run(context.Background(), RunInput{TaskID: 7, Kind: KindUserList, UserID: 9, Platform: "admin", Scope: ScopeSelected, IDs: []int64{3}})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if provider.input.TaskID != 7 || provider.input.Kind != KindUserList || provider.input.UserID != 9 || provider.input.Platform != "admin" || provider.input.Scope != ScopeSelected || len(provider.input.IDs) != 1 || provider.input.IDs[0] != 3 {
		t.Fatalf("unexpected provider input: %#v", provider.input)
	}
	if uploader.input.TaskID != 7 || uploader.input.ArtifactVersion != "v1" || !uploader.input.CreatedAt.Equal(createdAt) || string(uploader.input.Body) != "xlsx" || uploader.input.RowCount != 1 {
		t.Fatalf("unexpected upload input: %#v", uploader.input)
	}
	if repo.successResult.FileName != "u.xlsx" || repo.successResult.FileURL != "https://cos/u.xlsx" || repo.successResult.ObjectKey != "exports/user_list/20260507/u.xlsx" || repo.successID != 7 {
		t.Fatalf("unexpected success mark id=%d result=%#v", repo.successID, repo.successResult)
	}
	if notifier.success.TaskID != 7 || notifier.success.UserID != 9 || notifier.success.Link != "/system/exportTask?status=2" {
		t.Fatalf("unexpected success notification: %#v", notifier.success)
	}
}

func TestRunMarksFailedAndNotifiesWhenGenerationFails(t *testing.T) {
	repo := &fakeRepository{getRow: &Task{ID: 7, UserID: 9, Platform: enum.PlatformAdmin, Kind: KindUserList, Title: "用户列表导出", Status: enum.ExportTaskStatusPending, IsDel: enum.CommonNo, CreatedAt: time.Now()}}
	provider := &fakeDataProvider{err: errors.New("provider failed")}
	notifier := &fakeNotifier{}

	err := NewService(repo, WithDefinitionRegistry(testRegistry(t, provider)), WithFileWriter(&fakeFileWriter{}), WithFileUploader(&fakeUploader{}), WithNotifier(notifier)).Run(context.Background(), RunInput{TaskID: 7, Kind: KindUserList, UserID: 9, Scope: ScopeSelected, IDs: []int64{3}})
	if err == nil {
		t.Fatalf("expected Run error")
	}
	if repo.markFailedID != 7 || repo.failedMessage == "" {
		t.Fatalf("expected task failure mark, got id=%d msg=%q", repo.markFailedID, repo.failedMessage)
	}
	if notifier.failed.TaskID != 7 || notifier.failed.UserID != 9 || notifier.failed.Link != "/system/exportTask?status=3" {
		t.Fatalf("unexpected failed notification: %#v", notifier.failed)
	}
}

func TestRunMarksFailedForUnknownKind(t *testing.T) {
	repo := &fakeRepository{getRow: &Task{ID: 7, UserID: 9, Platform: enum.PlatformAdmin, Kind: "payment_orders", Title: "订单导出", Status: enum.ExportTaskStatusPending, IsDel: enum.CommonNo, CreatedAt: time.Now()}}
	notifier := &fakeNotifier{}

	err := NewService(repo, WithDefinitionRegistry(testRegistry(t, &fakeDataProvider{})), WithFileWriter(&fakeFileWriter{}), WithFileUploader(&fakeUploader{}), WithNotifier(notifier)).Run(context.Background(), RunInput{TaskID: 7, Kind: "payment_orders", UserID: 9, Scope: ScopeSelected, IDs: []int64{3}})
	if err == nil {
		t.Fatalf("expected Run error")
	}
	if repo.markFailedID != 7 || !strings.Contains(repo.failedMessage, "runtime is not configured") {
		t.Fatalf("expected unknown kind to mark failed, got id=%d msg=%q", repo.markFailedID, repo.failedMessage)
	}
	if notifier.failed.TaskID != 7 || notifier.failed.UserID != 9 {
		t.Fatalf("unexpected failed notification: %#v", notifier.failed)
	}
}

func TestLeaseLossCancelsExportWorkAndPreventsTerminalWrite(t *testing.T) {
	repo := &fakeRepository{
		getRow: &Task{
			ID: 11, UserID: 9, Platform: enum.PlatformAdmin, Kind: KindUserList,
			Title: "lease", Status: enum.ExportTaskStatusPending, IsDel: enum.CommonNo, CreatedAt: time.Now(),
		},
		renewLost: true,
	}
	provider := &fakeDataProvider{blockUntilCanceled: true}
	service := NewService(
		repo,
		WithDefinitionRegistry(testRegistry(t, provider)),
		WithFileWriter(&fakeFileWriter{}),
		WithFileUploader(&fakeUploader{}),
		WithWorkLease("worker-a", 15*time.Millisecond),
	)

	err := service.Run(context.Background(), RunInput{
		TaskID: 11, Kind: KindUserList, UserID: 9, Platform: enum.PlatformAdmin,
		Scope: ScopeSelected, IDs: []int64{1},
	})
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expected export lease loss, got %v", err)
	}
	if repo.terminalCalls != 0 {
		t.Fatalf("lease-lost export worker terminalized task %d times", repo.terminalCalls)
	}
}

func TestDuplicateExportPublicationUsesOneObjectKeyAndOneTerminalUpdate(t *testing.T) {
	createdAt := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	repository := &retryExportRepository{task: Task{
		ID: 42, UserID: 9, Platform: enum.PlatformAdmin, Kind: KindUserList,
		Title: "users", Status: enum.ExportTaskStatusPending, IsDel: enum.CommonNo, CreatedAt: createdAt,
	}}
	provider := &fakeDataProvider{data: &FileData{
		Prefix: "users", Headers: []Column{{Key: "id", Title: "ID"}}, Rows: []map[string]string{{"id": "1"}},
	}}
	objectWriter := &recordingCOSWriter{}
	uploader := NewCOSUploader(fakeUploadConfigRepository{config: &UploadConfig{
		Driver: enum.UploadDriverCOS, SecretIDEnc: "sid", SecretKeyEnc: "skey", Bucket: "bucket", Region: "ap-guangzhou",
	}}, plainSecretbox{}, objectWriter)
	notifier := &fakeNotifier{}
	service := NewService(
		repository,
		WithDefinitionRegistry(testRegistry(t, provider)),
		WithFileWriter(&fakeFileWriter{body: []byte("xlsx")}),
		WithFileUploader(uploader),
		WithNotifier(notifier),
		WithWorkLease("worker-a", time.Minute),
		WithNow(func() time.Time { return createdAt.Add(time.Second) }),
	)
	input := RunInput{TaskID: 42, Kind: KindUserList, UserID: 9, Platform: enum.PlatformAdmin, Scope: ScopeSelected, IDs: []int64{1}}

	if err := service.Run(context.Background(), input); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("first stale publication must lose lease, got %v", err)
	}
	if err := service.Run(context.Background(), input); err != nil {
		t.Fatalf("reclaimed publication failed: %v", err)
	}
	if len(objectWriter.keys) != 2 || objectWriter.keys[0] != "exports/20260717/42-v1.xlsx" || objectWriter.keys[1] != objectWriter.keys[0] {
		t.Fatalf("retry published different object keys: %#v", objectWriter.keys)
	}
	if repository.successfulTerminals != 1 || notifier.successCalls != 1 {
		t.Fatalf("duplicate publication terminalized more than once: terminals=%d notifications=%d", repository.successfulTerminals, notifier.successCalls)
	}
}

type retryExportRepository struct {
	Repository
	task                Task
	claimCount          int
	successfulTerminals int
}

func (r *retryExportRepository) ClaimNext(ctx context.Context, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	return r.ClaimByID(ctx, r.task.ID, owner, now, ttl)
}

func (r *retryExportRepository) ClaimByID(_ context.Context, _ int64, owner string, now time.Time, ttl time.Duration) (*Claim, error) {
	r.claimCount++
	if r.claimCount > 2 {
		return nil, nil
	}
	return &Claim{Task: r.task, Owner: owner, Token: uint64(r.claimCount), LeaseExpiresAt: now.Add(ttl)}, nil
}

func (r *retryExportRepository) Renew(context.Context, int64, string, uint64, time.Time, time.Time) (bool, error) {
	return true, nil
}

func (r *retryExportRepository) MarkSuccess(_ context.Context, _ int64, _ string, token uint64, _ time.Time, _ SuccessResult) (bool, error) {
	if token == 1 {
		return false, nil
	}
	r.successfulTerminals++
	return true, nil
}

func (r *retryExportRepository) MarkFailed(context.Context, int64, string, uint64, time.Time, string) (bool, error) {
	return false, nil
}

func TestClaimExportWorkIsExclusiveReclaimableAndFenced(t *testing.T) {
	first, second := exportLeaseRepositories(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	expires := now.Add(7 * 24 * time.Hour)
	id, err := first.Create(context.Background(), Task{
		UserID: 9, Platform: enum.PlatformAdmin, Title: "lease test", Kind: KindUserList,
		Status: enum.ExportTaskStatusPending, ExpireAt: &expires, IsDel: enum.CommonNo,
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create export task: %v", err)
	}

	start := make(chan struct{})
	claims := make(chan *Claim, 2)
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for _, worker := range []struct {
		repository *GormRepository
		owner      string
	}{{first, "worker-a"}, {second, "worker-b"}} {
		workers.Add(1)
		go func(repository *GormRepository, owner string) {
			defer workers.Done()
			<-start
			claim, claimErr := repository.ClaimNext(context.Background(), owner, now, time.Minute)
			claims <- claim
			errs <- claimErr
		}(worker.repository, worker.owner)
	}
	close(start)
	workers.Wait()
	close(claims)
	close(errs)
	for claimErr := range errs {
		if claimErr != nil {
			t.Fatalf("concurrent export claim: %v", claimErr)
		}
	}
	var winner *Claim
	claimCount := 0
	for claim := range claims {
		if claim != nil {
			winner = claim
			claimCount++
		}
	}
	if claimCount != 1 || winner == nil || winner.Task.ID != id || winner.Token == 0 {
		t.Fatalf("expected exactly one export claim, count=%d winner=%#v", claimCount, winner)
	}
	result := SuccessResult{FileName: "7-v1.xlsx", FileURL: "https://cdn/7-v1.xlsx", ObjectKey: "exports/20260717/7-v1.xlsx", FileSize: 4, RowCount: 1}
	if ok, err := first.MarkSuccess(context.Background(), id, "stale-owner", winner.Token, now.Add(time.Second), result); err != nil || ok {
		t.Fatalf("stale owner terminalized export: ok=%v err=%v", ok, err)
	}
	renewedUntil := winner.LeaseExpiresAt.Add(time.Minute)
	if ok, err := first.Renew(context.Background(), id, winner.Owner, winner.Token, now.Add(time.Second), renewedUntil); err != nil || !ok {
		t.Fatalf("current export owner could not renew: ok=%v err=%v", ok, err)
	}
	if ok, err := second.Renew(context.Background(), id, "stale-owner", winner.Token, now.Add(time.Second), renewedUntil); err != nil || ok {
		t.Fatalf("stale export owner renewed lease: ok=%v err=%v", ok, err)
	}
	reclaimed, err := second.ClaimNext(context.Background(), "worker-c", renewedUntil.Add(time.Microsecond), time.Minute)
	if err != nil {
		t.Fatalf("reclaim expired export task: %v", err)
	}
	if reclaimed == nil || reclaimed.Token <= winner.Token {
		t.Fatalf("expected higher export fencing token: first=%#v second=%#v", winner, reclaimed)
	}
	if ok, err := first.MarkSuccess(context.Background(), id, winner.Owner, winner.Token, reclaimed.LeaseExpiresAt.Add(-time.Second), result); err != nil || ok {
		t.Fatalf("stale export token terminalized work: ok=%v err=%v", ok, err)
	}
	if ok, err := second.MarkSuccess(context.Background(), id, reclaimed.Owner, reclaimed.Token, reclaimed.LeaseExpiresAt.Add(-time.Second), result); err != nil || !ok {
		t.Fatalf("current export owner could not terminalize: ok=%v err=%v", ok, err)
	}
}

func TestCleanupExpiredSkipsActivelyLeasedExport(t *testing.T) {
	repository, _ := exportLeaseRepositories(t)
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	expiredAt := now.Add(-time.Hour)
	leaseExpiresAt := now.Add(time.Minute)
	owner := "worker-a"
	id, err := repository.Create(context.Background(), Task{
		UserID: 9, Platform: enum.PlatformAdmin, Title: "active", Kind: KindUserList,
		Status: enum.ExportTaskStatusPending, ClaimOwner: &owner, ClaimToken: 1,
		ClaimExpiresAt: &leaseExpiresAt, ExpireAt: &expiredAt, IsDel: enum.CommonNo,
		CreatedAt: now.Add(-24 * time.Hour), UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create actively leased export: %v", err)
	}
	if err := repository.CleanExpired(context.Background(), now); err != nil {
		t.Fatalf("cleanup with active lease: %v", err)
	}
	if row, err := repository.Get(context.Background(), id); err != nil || row == nil {
		t.Fatalf("active lease was deleted: row=%#v err=%v", row, err)
	}
	if err := repository.CleanExpired(context.Background(), leaseExpiresAt.Add(time.Microsecond)); err != nil {
		t.Fatalf("cleanup after lease expiry: %v", err)
	}
	if row, err := repository.Get(context.Background(), id); err != nil || row != nil {
		t.Fatalf("expired unleased export was not deleted: row=%#v err=%v", row, err)
	}
}

func exportLeaseRepositories(t *testing.T) (*GormRepository, *GormRepository) {
	t.Helper()
	dsn := os.Getenv("TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is required for export lease integration tests")
	}
	base, err := database.Open(config.MySQLConfig{DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatalf("open base MySQL: %v", err)
	}
	databaseName := fmt.Sprintf("p05_export_%d", time.Now().UnixNano())
	if err := base.Gorm.Exec("CREATE DATABASE `" + databaseName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci").Error; err != nil {
		_ = base.Close()
		t.Fatalf("create export test database: %v", err)
	}
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("parse MySQL DSN: %v", err)
	}
	parsed.DBName = databaseName
	testConfig := config.MySQLConfig{DSN: parsed.FormatDSN(), MaxOpenConns: 4, MaxIdleConns: 2, ConnMaxLifetime: time.Minute}
	clientA, err := database.Open(testConfig)
	if err != nil {
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("open first export test client: %v", err)
	}
	clientB, err := database.Open(testConfig)
	if err != nil {
		_ = clientA.Close()
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("open second export test client: %v", err)
	}
	if err := clientA.Gorm.Exec("CREATE TABLE export_tasks LIKE admin.export_tasks").Error; err != nil {
		_ = clientB.Close()
		_ = clientA.Close()
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
		t.Fatalf("create export test table: %v", err)
	}
	t.Cleanup(func() {
		_ = clientB.Close()
		_ = clientA.Close()
		_ = base.Gorm.Exec("DROP DATABASE `" + databaseName + "`").Error
		_ = base.Close()
	})
	return &GormRepository{db: clientA.Gorm}, &GormRepository{db: clientB.Gorm}
}
