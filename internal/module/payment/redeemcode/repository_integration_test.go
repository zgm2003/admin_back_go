package redeemcode

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/module/payment/wallet"
	"admin_back_go/internal/shared/enum"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestConcurrentBatchRequestReplayCreatesOneBatch(t *testing.T) {
	client, closeDB := openRedeemIntegrationDB(t)
	defer closeDB()
	fixture := newRedeemIntegrationFixture(t, client)
	defer fixture.cleanup(t)

	now := time.Now().UTC().Truncate(time.Microsecond)
	repository := NewGormRepository(client, wallet.NewGormRepository(client))
	first := fixture.batchRecord(t, now, 2)
	second := first
	second.Batch.BatchNo = first.Batch.BatchNo + "B"
	second.Codes = integrationCodes(t, 2)

	results := make(chan *BatchWithCodes, 2)
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for _, record := range []CreateBatchRecord{first, second} {
		wait.Add(1)
		go func(candidate CreateBatchRecord) {
			defer wait.Done()
			batch, _, err := repository.CreateOrReplayBatch(context.Background(), candidate)
			results <- batch
			errorsSeen <- err
		}(record)
	}
	wait.Wait()
	close(results)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent create error: %v", err)
		}
	}
	var batchID int64
	for batch := range results {
		if batch == nil || len(batch.Codes) != 2 {
			t.Fatalf("batch=%+v", batch)
		}
		if batchID == 0 {
			batchID = batch.Batch.ID
		} else if batch.Batch.ID != batchID {
			t.Fatalf("request replay produced batch IDs %d and %d", batchID, batch.Batch.ID)
		}
	}
	fixture.batchIDs = append(fixture.batchIDs, batchID)
	assertIntegrationCount(t, client.SQL, "SELECT COUNT(*) FROM redeem_code_batches WHERE created_by=? AND request_id=?", 1, fixture.creatorID, fixture.requestID)
	assertIntegrationCount(t, client.SQL, "SELECT COUNT(*) FROM redeem_codes WHERE batch_id=?", 2, batchID)
}

func TestConcurrentRedeemHasOneWalletCredit(t *testing.T) {
	client, closeDB := openRedeemIntegrationDB(t)
	defer closeDB()
	fixture := newRedeemIntegrationFixture(t, client)
	defer fixture.cleanup(t)
	fixture.userIDs = append(fixture.userIDs, fixture.createUser(t), fixture.createUser(t))

	repository := NewGormRepository(client, wallet.NewGormRepository(client))
	created, _, err := repository.CreateOrReplayBatch(context.Background(), fixture.batchRecord(t, time.Now().UTC(), 1))
	if err != nil {
		t.Fatal(err)
	}
	fixture.batchIDs = append(fixture.batchIDs, created.Batch.ID)
	code := created.Codes[0]

	results := make(chan error, 2)
	var wait sync.WaitGroup
	for _, userID := range fixture.userIDs {
		wait.Add(1)
		go func(id int64) {
			defer wait.Done()
			_, redeemErr := repository.Redeem(context.Background(), id, code.Code)
			results <- redeemErr
		}(userID)
	}
	wait.Wait()
	close(results)
	successes, unavailable := 0, 0
	for redeemErr := range results {
		switch {
		case redeemErr == nil:
			successes++
		case errors.Is(redeemErr, ErrUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected redeem error: %v", redeemErr)
		}
	}
	if successes != 1 || unavailable != 1 {
		t.Fatalf("successes=%d unavailable=%d", successes, unavailable)
	}
	assertIntegrationCount(t, client.SQL, "SELECT COUNT(*) FROM wallet_transactions WHERE source_type=? AND source_id=? AND is_del=?", 1, wallet.SourceRedeemCode, code.ID, enum.CommonNo)
}

func TestConcurrentRedeemAndVoidHasOneTerminalState(t *testing.T) {
	client, closeDB := openRedeemIntegrationDB(t)
	defer closeDB()
	fixture := newRedeemIntegrationFixture(t, client)
	defer fixture.cleanup(t)
	userID := fixture.createUser(t)
	fixture.userIDs = append(fixture.userIDs, userID)

	repository := NewGormRepository(client, wallet.NewGormRepository(client))
	created, _, err := repository.CreateOrReplayBatch(context.Background(), fixture.batchRecord(t, time.Now().UTC(), 1))
	if err != nil {
		t.Fatal(err)
	}
	fixture.batchIDs = append(fixture.batchIDs, created.Batch.ID)
	code := created.Codes[0]

	start := make(chan struct{})
	redeemResult := make(chan error, 1)
	voidResult := make(chan error, 1)
	go func() {
		<-start
		_, redeemErr := repository.Redeem(context.Background(), userID, code.Code)
		redeemResult <- redeemErr
	}()
	go func() {
		<-start
		_, voidErr := repository.VoidCodes(context.Background(), []int64{code.ID}, time.Now().UTC())
		voidResult <- voidErr
	}()
	close(start)
	redeemErr, voidErr := <-redeemResult, <-voidResult
	if (redeemErr == nil) == (voidErr == nil) {
		t.Fatalf("exactly one operation must win: redeem=%v void=%v", redeemErr, voidErr)
	}
	if redeemErr != nil && !errors.Is(redeemErr, ErrUnavailable) {
		t.Fatalf("redeem loser error=%v", redeemErr)
	}
	if voidErr != nil && !errors.Is(voidErr, ErrVoidConflict) {
		t.Fatalf("void loser error=%v", voidErr)
	}
	var state string
	if err := client.SQL.QueryRow("SELECT state FROM redeem_codes WHERE id=?", code.ID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	var credits int
	if err := client.SQL.QueryRow("SELECT COUNT(*) FROM wallet_transactions WHERE source_type=? AND source_id=? AND is_del=?", wallet.SourceRedeemCode, code.ID, enum.CommonNo).Scan(&credits); err != nil {
		t.Fatal(err)
	}
	if (state == StateUsed && credits != 1) || (state == StateVoided && credits != 0) || (state != StateUsed && state != StateVoided) {
		t.Fatalf("terminal state=%q credits=%d", state, credits)
	}
}

type redeemIntegrationFixture struct {
	client    *database.Client
	prefix    string
	creatorID int64
	userIDs   []int64
	batchIDs  []int64
	requestID string
	userSeq   int
}

func openRedeemIntegrationDB(t *testing.T) (*database.Client, func()) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("TEST_MYSQL_DSN"))
	if dsn == "" {
		t.Skip("TEST_MYSQL_DSN is not set")
	}
	parsed, err := mysqlDriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse TEST_MYSQL_DSN: %v", err)
	}
	databaseName := strings.ToLower(strings.TrimSpace(parsed.DBName))
	if databaseName == "" || !strings.Contains(databaseName, "test") || databaseName == "mysql" || databaseName == "information_schema" || databaseName == "performance_schema" || databaseName == "sys" {
		t.Fatalf("TEST_MYSQL_DSN must target a dedicated test database, got %q", parsed.DBName)
	}
	sqlDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		t.Fatalf("ping test MySQL: %v", err)
	}
	gormDB, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		_ = sqlDB.Close()
		t.Fatal(err)
	}
	return &database.Client{Gorm: gormDB, SQL: sqlDB}, func() { _ = sqlDB.Close() }
}

func newRedeemIntegrationFixture(t *testing.T, client *database.Client) *redeemIntegrationFixture {
	t.Helper()
	prefix := fmt.Sprintf("rc_it_%d", time.Now().UnixNano())
	fixture := &redeemIntegrationFixture{client: client, prefix: prefix, requestID: prefix + "_request"}
	fixture.creatorID = fixture.createUser(t)
	return fixture
}

func (fixture *redeemIntegrationFixture) createUser(t *testing.T) int64 {
	t.Helper()
	fixture.userSeq++
	username := fmt.Sprintf("%s_%d", fixture.prefix, fixture.userSeq)
	result, err := fixture.client.SQL.Exec("INSERT INTO users (username, status, is_del) VALUES (?, 1, ?)", username, enum.CommonNo)
	if err != nil {
		t.Fatalf("insert fixture user: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (fixture *redeemIntegrationFixture) batchRecord(t *testing.T, now time.Time, quantity int) CreateBatchRecord {
	t.Helper()
	expires := now.UTC().Truncate(time.Microsecond).Add(time.Hour)
	fingerprint, err := batchRequestFingerprint(100, quantity, &expires, fixture.prefix)
	if err != nil {
		t.Fatal(err)
	}
	return CreateBatchRecord{Batch: Batch{
		BatchNo:   fmt.Sprintf("RCB%s%d", now.UTC().Format("20060102150405"), time.Now().UnixNano()),
		RequestID: fixture.requestID, RequestFingerprintVersion: RequestFingerprintVersion, RequestFingerprint: fingerprint,
		AmountCents: 100, Quantity: quantity, ExpiresAt: &expires, Note: fixture.prefix, CreatedBy: fixture.creatorID,
	}, Codes: integrationCodes(t, quantity)}
}

func integrationCodes(t *testing.T, quantity int) []Code {
	t.Helper()
	codes := make([]Code, quantity)
	seen := make(map[string]struct{}, quantity)
	for index := range codes {
		for {
			code, err := GenerateCode(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			codes[index] = Code{Code: code, State: StateUnused}
			break
		}
	}
	return codes
}

func (fixture *redeemIntegrationFixture) cleanup(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	transaction, err := fixture.client.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Errorf("begin cleanup: %v", err)
		return
	}
	defer transaction.Rollback()
	if _, err := transaction.ExecContext(ctx, "DELETE wt FROM wallet_transactions wt JOIN redeem_codes rc ON rc.id=wt.source_id WHERE wt.source_type=? AND rc.batch_id IN (SELECT id FROM redeem_code_batches WHERE created_by=? AND request_id=?)", wallet.SourceRedeemCode, fixture.creatorID, fixture.requestID); err != nil {
		t.Errorf("delete transactions: %v", err)
		return
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM redeem_codes WHERE batch_id IN (SELECT id FROM redeem_code_batches WHERE created_by=? AND request_id=?)", fixture.creatorID, fixture.requestID); err != nil {
		t.Errorf("delete codes: %v", err)
		return
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM redeem_code_batches WHERE created_by=? AND request_id=?", fixture.creatorID, fixture.requestID); err != nil {
		t.Errorf("delete batches: %v", err)
		return
	}
	allUsers := append([]int64{fixture.creatorID}, fixture.userIDs...)
	if len(allUsers) > 0 {
		placeholders := strings.TrimRight(strings.Repeat("?,", len(allUsers)), ",")
		arguments := make([]any, len(allUsers))
		for index, id := range allUsers {
			arguments[index] = id
		}
		if _, err := transaction.ExecContext(ctx, "DELETE FROM user_wallets WHERE user_id IN ("+placeholders+")", arguments...); err != nil {
			t.Errorf("delete wallets: %v", err)
			return
		}
		if _, err := transaction.ExecContext(ctx, "DELETE FROM users WHERE id IN ("+placeholders+")", arguments...); err != nil {
			t.Errorf("delete users: %v", err)
			return
		}
	}
	if err := transaction.Commit(); err != nil {
		t.Errorf("commit cleanup: %v", err)
	}
}

func assertIntegrationCount(t *testing.T, db *sql.DB, query string, want int, arguments ...any) {
	t.Helper()
	var count int
	if err := db.QueryRow(query, arguments...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("count=%d want %d", count, want)
	}
}
