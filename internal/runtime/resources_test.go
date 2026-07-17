package runtime

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/redisclient"
)

func TestOpenResourcesWithStartupRetryClosesPartialAttemptsBeforePublishing(t *testing.T) {
	var events []string
	openers := successfulOpeners(&events)
	redisAttempts := 0
	databaseCloses := 0

	openers.Database = func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
		opened := openedDatabase(&events, "database")
		closeResource := opened.Close
		opened.Close = func(ctx context.Context) error {
			databaseCloses++
			return closeResource(ctx)
		}
		return opened, nil
	}
	openers.Redis = func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
		redisAttempts++
		if redisAttempts < 3 {
			return OpenedResource[*redisclient.Client]{}, errors.New("redis unavailable")
		}
		return openedRedis(&events, "redis"), nil
	}

	waits := 0
	resources, err := openResourcesWithStartupRetry(
		t.Context(),
		config.ProcessAPI,
		configuredResources(),
		openers,
		resourceRetryPolicy{
			Attempts: 3,
			Wait: func(context.Context, time.Duration) error {
				waits++
				return nil
			},
		},
	)
	if err != nil || resources == nil {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	defer resources.Close(t.Context())
	if redisAttempts != 3 || waits != 2 || databaseCloses != 2 {
		t.Fatalf("redis attempts=%d waits=%d database closes=%d", redisAttempts, waits, databaseCloses)
	}
}

func TestOpenResourcesWithStartupRetryReturnsLastErrorAtAttemptBound(t *testing.T) {
	dependencyErr := errors.New("database unavailable")
	attempts := 0
	waits := 0
	openers := Openers{
		Database: func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
			attempts++
			return OpenedResource[*database.Client]{}, dependencyErr
		},
	}

	resources, err := openResourcesWithStartupRetry(
		t.Context(),
		config.ProcessAPI,
		configuredResources(),
		openers,
		resourceRetryPolicy{
			Attempts: 3,
			Wait: func(context.Context, time.Duration) error {
				waits++
				return nil
			},
		},
	)
	if resources != nil || !errors.Is(err, dependencyErr) {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	if attempts != 3 || waits != 2 {
		t.Fatalf("attempts=%d waits=%d", attempts, waits)
	}
}

func TestOpenResourcesWithStartupRetryDoesNotRetryPermanentFailure(t *testing.T) {
	cfg := configuredResources()
	cfg.MySQL.DSN = ""
	waits := 0

	resources, err := openResourcesWithStartupRetry(
		t.Context(),
		config.ProcessAPI,
		cfg,
		Openers{},
		resourceRetryPolicy{
			Attempts: 3,
			Wait: func(context.Context, time.Duration) error {
				waits++
				return nil
			},
		},
	)
	if resources != nil || err == nil || !strings.Contains(err.Error(), "database configuration") {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	if waits != 0 {
		t.Fatalf("permanent failure waits=%d", waits)
	}
}

func TestOpenResourcesWithStartupRetryStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	openers := Openers{
		Database: func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
			return OpenedResource[*database.Client]{}, errors.New("database unavailable")
		},
	}
	waits := 0

	resources, err := openResourcesWithStartupRetry(
		ctx,
		config.ProcessAPI,
		configuredResources(),
		openers,
		resourceRetryPolicy{
			Attempts: 3,
			Delay:    time.Hour,
			Wait: func(ctx context.Context, delay time.Duration) error {
				waits++
				cancel()
				return waitForResourceRetry(ctx, delay)
			},
		},
	)
	if resources != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	if waits != 1 {
		t.Fatalf("waits=%d", waits)
	}
}

func TestOpenResourcesClosesDatabaseWhenRedisOpenFails(t *testing.T) {
	redisErr := errors.New("redis open failed")
	var events []string
	openers := Openers{
		Database: func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
			events = append(events, "open:database")
			return OpenedResource[*database.Client]{
				Client: &database.Client{},
				Ping: func(context.Context) error {
					events = append(events, "ping:database")
					return nil
				},
				Close: func(context.Context) error {
					events = append(events, "close:database")
					return nil
				},
			}, nil
		},
		Redis: func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
			events = append(events, "open:redis")
			return OpenedResource[*redisclient.Client]{}, redisErr
		},
	}

	resources, err := OpenResources(t.Context(), config.ProcessAPI, configuredResources(), openers)
	if resources != nil {
		t.Fatalf("partial resources published: %+v", resources)
	}
	if !errors.Is(err, redisErr) {
		t.Fatalf("redis cause missing: %v", err)
	}
	want := []string{"open:database", "ping:database", "open:redis", "close:database"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
}

func TestOpenResourcesClosesCurrentResourceWhenPingFails(t *testing.T) {
	pingErr := errors.New("database ping failed")
	closed := 0
	openers := Openers{
		Database: func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
			return OpenedResource[*database.Client]{
				Client: &database.Client{},
				Ping:   func(context.Context) error { return pingErr },
				Close: func(context.Context) error {
					closed++
					return nil
				},
			}, nil
		},
	}

	resources, err := OpenResources(t.Context(), config.ProcessAPI, configuredResources(), openers)
	if resources != nil || !errors.Is(err, pingErr) {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	if closed != 1 {
		t.Fatalf("database close count=%d", closed)
	}
}

func TestOpenResourcesCleansSuccessfulOpenerThatReturnsNilClient(t *testing.T) {
	closed := 0
	openers := Openers{
		Database: func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
			return OpenedResource[*database.Client]{
				Close: func(context.Context) error {
					closed++
					return nil
				},
			}, nil
		},
	}

	resources, err := OpenResources(t.Context(), config.ProcessAPI, configuredResources(), openers)
	if resources != nil || err == nil {
		t.Fatalf("resources=%+v err=%v", resources, err)
	}
	if closed != 1 {
		t.Fatalf("broken successful opener close count=%d", closed)
	}
}

func TestOpenResourcesAPIPlanOpensRequiredResourcesAndReportsDisabledCapabilities(t *testing.T) {
	var events []string
	openers := successfulOpeners(&events)
	cfg := configuredResources()
	cfg.Queue.Enabled = false
	cfg.Realtime.Enabled = false
	cfg.Scheduler.Enabled = true

	resources, err := OpenResources(t.Context(), config.ProcessAPI, cfg, openers)
	if err != nil {
		t.Fatalf("open resources: %v", err)
	}
	if resources.DB == nil || resources.Redis == nil || resources.TokenRedis == nil {
		t.Fatalf("API required resources missing: %+v", resources)
	}
	if resources.QueueRedis != nil {
		t.Fatalf("queue redis should be disabled: %+v", resources.QueueRedis)
	}

	report := resources.Health(t.Context())
	for _, name := range []string{"database", "redis", "token_redis"} {
		if got := report.Checks[name].Status; got != StatusUp {
			t.Fatalf("%s status=%q report=%+v", name, got, report)
		}
	}
	for _, name := range []string{"queue_redis", "realtime", "scheduler"} {
		if got := report.Checks[name].Status; got != StatusDisabled {
			t.Fatalf("%s status=%q report=%+v", name, got, report)
		}
	}
	if report.Status != StatusReady {
		t.Fatalf("report=%+v", report)
	}

	if err := resources.Close(t.Context()); err != nil {
		t.Fatalf("close resources: %v", err)
	}
	if err := resources.Close(t.Context()); err != nil {
		t.Fatalf("second close: %v", err)
	}
	closeEvents := filterEvents(events, "close:")
	if want := []string{"close:token_redis", "close:redis", "close:database"}; !reflect.DeepEqual(closeEvents, want) {
		t.Fatalf("close events=%v want=%v", closeEvents, want)
	}
}

func TestOpenResourcesWorkerPlanSkipsTokenRedisAndRequiresQueue(t *testing.T) {
	var events []string
	openers := successfulOpeners(&events)
	openers.TokenRedis = func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
		t.Fatal("worker must not open token redis")
		return OpenedResource[*redisclient.Client]{}, nil
	}
	cfg := configuredResources()
	cfg.Queue.Enabled = true
	cfg.Realtime.Enabled = true
	cfg.Realtime.Publisher = config.RealtimePublisherRedis
	cfg.Scheduler.Enabled = true

	resources, err := OpenResources(t.Context(), config.ProcessWorker, cfg, openers)
	if err != nil {
		t.Fatalf("open resources: %v", err)
	}
	defer resources.Close(t.Context())
	if resources.TokenRedis != nil || resources.QueueRedis == nil {
		t.Fatalf("worker resource graph mismatch: %+v", resources)
	}
	report := resources.Health(t.Context())
	if report.Checks["token_redis"].Status != StatusDisabled || report.Checks["queue_redis"].Status != StatusUp {
		t.Fatalf("worker redis capability report=%+v", report)
	}
	if report.Checks["realtime"].Status != StatusUp || report.Checks["scheduler"].Status != StatusUp {
		t.Fatalf("worker enabled capability report=%+v", report)
	}
}

func TestOpenResourcesRejectsEnabledRedisCapabilitiesWithoutAddress(t *testing.T) {
	tests := []struct {
		name    string
		process config.Process
		mutate  func(*config.Config)
	}{
		{name: "queue", process: config.ProcessAPI, mutate: func(cfg *config.Config) { cfg.Queue.Enabled = true }},
		{name: "realtime", process: config.ProcessAPI, mutate: func(cfg *config.Config) { cfg.Realtime.Enabled = true }},
		{name: "scheduler", process: config.ProcessWorker, mutate: func(cfg *config.Config) {
			cfg.Queue.Enabled = true
			cfg.Scheduler.Enabled = true
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configuredResources()
			cfg.Redis.Addr = ""
			cfg.Queue.Enabled = false
			cfg.Realtime.Enabled = false
			cfg.Scheduler.Enabled = false
			tc.mutate(&cfg)
			resources, err := OpenResources(t.Context(), tc.process, cfg, Openers{})
			if resources != nil || err == nil || !strings.Contains(err.Error(), "redis") {
				t.Fatalf("resources=%+v err=%v", resources, err)
			}
		})
	}
}

func configuredResources() config.Config {
	return config.Config{
		MySQL: config.MySQLConfig{DSN: "user:password@tcp(mysql:3306)/admin"},
		Redis: config.RedisConfig{Addr: "redis:6379"},
		Token: config.TokenConfig{RedisDB: 2},
		Queue: config.QueueConfig{RedisDB: 3},
	}
}

func successfulOpeners(events *[]string) Openers {
	return Openers{
		Database: func(context.Context, config.MySQLConfig) (OpenedResource[*database.Client], error) {
			return openedDatabase(events, "database"), nil
		},
		Redis: func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
			return openedRedis(events, "redis"), nil
		},
		TokenRedis: func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
			return openedRedis(events, "token_redis"), nil
		},
		QueueRedis: func(context.Context, config.RedisConfig) (OpenedResource[*redisclient.Client], error) {
			return openedRedis(events, "queue_redis"), nil
		},
	}
}

func openedDatabase(events *[]string, name string) OpenedResource[*database.Client] {
	*events = append(*events, "open:"+name)
	return OpenedResource[*database.Client]{
		Client: &database.Client{},
		Ping: func(context.Context) error {
			*events = append(*events, "ping:"+name)
			return nil
		},
		Close: func(context.Context) error {
			*events = append(*events, "close:"+name)
			return nil
		},
	}
}

func openedRedis(events *[]string, name string) OpenedResource[*redisclient.Client] {
	*events = append(*events, "open:"+name)
	return OpenedResource[*redisclient.Client]{
		Client: &redisclient.Client{},
		Ping: func(context.Context) error {
			*events = append(*events, "ping:"+name)
			return nil
		},
		Close: func(context.Context) error {
			*events = append(*events, "close:"+name)
			return nil
		},
	}
}

func filterEvents(events []string, prefix string) []string {
	filtered := make([]string, 0, len(events))
	for _, event := range events {
		if strings.HasPrefix(event, prefix) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
