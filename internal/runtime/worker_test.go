package runtime

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	infrarealtime "admin_back_go/internal/infra/realtime"
	"admin_back_go/internal/jobs"
	"admin_back_go/internal/module/ai/contextengine"
	"admin_back_go/internal/module/crontask"
)

func TestWorkerReadinessRequiresExactlyPlan04TurnTaskRegistrations(t *testing.T) {
	registry, err := jobs.NewRegistry(jobs.Dependencies{ContextDocumentIndex: contextDocumentIndexStub{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := requireContextTaskRegistrations(registry); err != nil {
		t.Fatal(err)
	}
	var contextTypes []string
	for _, taskType := range registry.Types() {
		if strings.HasPrefix(taskType, "ai:context-") {
			contextTypes = append(contextTypes, taskType)
		}
	}
	want := []string{contextengine.TaskContextConversationIndexV1, contextengine.TaskContextDocumentIndexV1, contextengine.TaskContextIndexCleanupV1, contextengine.TaskContextProfileRebuildV1}
	slices.Sort(want)
	if !slices.Equal(contextTypes, want) {
		t.Fatalf("context task types=%v want=%v", contextTypes, want)
	}
}

type contextDocumentIndexStub struct{}

func (contextDocumentIndexStub) IndexDocument(context.Context, uint64) (contextengine.DocumentIndexAttempt, error) {
	return contextengine.DocumentIndexAttempt{}, nil
}
func (contextDocumentIndexStub) FinalizeDocumentIndex(context.Context, contextengine.DocumentIndexAttempt, string, int) error {
	return nil
}

func TestNewWorkerValidatesSecretsWithoutOpeningResources(t *testing.T) {
	runtime, err := NewWorker(config.Config{}, slog.Default())
	if runtime != nil || err == nil || !strings.Contains(err.Error(), "APP_SECRET") {
		t.Fatalf("runtime=%+v err=%v", runtime, err)
	}

	runtime, err = NewWorker(config.Config{
		App: config.AppConfig{Secret: strings.Repeat("a", 64)},
	}, slog.Default())
	if err != nil || runtime == nil {
		t.Fatalf("constructor should not open external resources: runtime=%+v err=%v", runtime, err)
	}
}

func TestWorkerRuntimeStartsAndStopsInLifecycleOrder(t *testing.T) {
	var mu sync.Mutex
	events := make([]string, 0, 10)
	record := func(event string) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}
	ready := make(chan struct{})
	hooks := workerHooks{
		openResources: func(context.Context) (func(context.Context) Report, CleanupFunc, error) {
			record("start:resources")
			return nil, func(context.Context) error {
				record("stop:resources")
				return nil
			}, nil
		},
		buildProviders: lifecycleHook(record, "providers", "queue_producer"),
		buildHandlers:  lifecycleHook(record, "task_handlers", "publisher"),
		startQueue:     lifecycleHook(record, "queue", "queue"),
		startScheduler: func(context.Context) (CleanupFunc, error) {
			record("start:scheduler")
			close(ready)
			return func(context.Context) error {
				record("stop:scheduler")
				return nil
			}, nil
		},
	}
	runtime := newWorkerRuntimeWithHooks(hooks)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() { started <- runtime.Start(ctx) }()
	select {
	case <-ready:
	case <-t.Context().Done():
		t.Fatal("worker did not start")
	}
	cancel()
	if err := <-started; err != nil {
		t.Fatalf("Start returned error after cancellation: %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	want := []string{
		"start:resources",
		"start:providers",
		"start:task_handlers",
		"start:queue",
		"start:scheduler",
		"stop:scheduler",
		"stop:queue",
		"stop:publisher",
		"stop:queue_producer",
		"stop:resources",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v\nwant=%v", got, want)
	}
}

func TestWorkerRuntimeRejectsConcurrentSecondStart(t *testing.T) {
	ready := make(chan struct{})
	runtime := newWorkerRuntimeWithHooks(workerHooks{
		startScheduler: func(context.Context) (CleanupFunc, error) {
			close(ready)
			return nil, nil
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() { started <- runtime.Start(ctx) }()
	select {
	case <-ready:
	case <-t.Context().Done():
		t.Fatal("worker did not start")
	}
	if err := runtime.Start(context.Background()); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start error=%v, want %v", err, ErrAlreadyStarted)
	}
	cancel()
	if err := <-started; err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

func TestWorkerRuntimeCleansAfterFailureAtEveryBoundary(t *testing.T) {
	boundaries := []string{"resources", "providers", "handlers", "queue", "scheduler"}
	for _, failAt := range boundaries {
		t.Run(failAt, func(t *testing.T) {
			failure := errors.New("fail at " + failAt)
			var stopped []string
			step := func(name string) func(context.Context) (CleanupFunc, error) {
				return func(context.Context) (CleanupFunc, error) {
					if failAt == name {
						return nil, failure
					}
					return func(context.Context) error {
						stopped = append(stopped, name)
						return nil
					}, nil
				}
			}
			hooks := workerHooks{
				openResources: func(context.Context) (func(context.Context) Report, CleanupFunc, error) {
					if failAt == "resources" {
						return nil, nil, failure
					}
					return nil, func(context.Context) error {
						stopped = append(stopped, "resources")
						return nil
					}, nil
				},
				buildProviders: step("providers"),
				buildHandlers:  step("handlers"),
				startQueue:     step("queue"),
				startScheduler: step("scheduler"),
			}
			err := newWorkerRuntimeWithHooks(hooks).Start(context.Background())
			if !errors.Is(err, failure) {
				t.Fatalf("Start error=%v, want %v", err, failure)
			}
			failureIndex := 0
			for index, name := range boundaries {
				if name == failAt {
					failureIndex = index
					break
				}
			}
			wantStopped := append([]string(nil), boundaries[:failureIndex]...)
			for left, right := 0, len(wantStopped)-1; left < right; left, right = left+1, right-1 {
				wantStopped[left], wantStopped[right] = wantStopped[right], wantStopped[left]
			}
			if !reflect.DeepEqual(stopped, wantStopped) {
				t.Fatalf("stopped=%v want=%v", stopped, wantStopped)
			}
		})
	}
}

func TestWorkerRuntimeHealthRemainsStoppedAfterShutdown(t *testing.T) {
	ready := make(chan struct{})
	runtime := newWorkerRuntimeWithHooks(workerHooks{
		openResources: func(context.Context) (func(context.Context) Report, CleanupFunc, error) {
			return func(context.Context) Report {
				return NewReport(map[string]Check{"database": {Status: StatusUp}})
			}, nil, nil
		},
		startScheduler: func(context.Context) (CleanupFunc, error) {
			close(ready)
			return nil, nil
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Start(ctx) }()
	<-ready
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	report := runtime.Health(context.Background())
	if report.Status != StatusNotReady || report.Checks["runtime"].Status != StatusDown {
		t.Fatalf("stopped runtime reported healthy: %+v", report)
	}
}

func TestWorkerRuntimeClosesStepThatFinishesDuringShutdown(t *testing.T) {
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	handlerClosed := make(chan struct{})
	runtime := newWorkerRuntimeWithHooks(workerHooks{
		buildHandlers: func(context.Context) (CleanupFunc, error) {
			close(handlerStarted)
			<-releaseHandler
			return func(context.Context) error {
				close(handlerClosed)
				return nil
			}, nil
		},
	})
	started := make(chan error, 1)
	go func() { started <- runtime.Start(context.Background()) }()
	<-handlerStarted
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	close(releaseHandler)
	if err := <-started; !errors.Is(err, ErrCleanupClosed) {
		t.Fatalf("Start error=%v, want %v", err, ErrCleanupClosed)
	}
	select {
	case <-handlerClosed:
	case <-time.After(time.Second):
		t.Fatal("late handler cleanup was not executed")
	}
}

func TestRealtimePublisherForWorkerUsesRedisOnlyForCrossProcessFanout(t *testing.T) {
	publisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{
			Enabled:      true,
			Publisher:    config.RealtimePublisherRedis,
			RedisChannel: "admin_go:realtime:test",
		},
	}, &Resources{})
	if _, ok := publisher.(*infrarealtime.RedisPublisher); !ok {
		t.Fatalf("expected worker redis publisher, got %T", publisher)
	}
}

func TestWorkerReplyRepositoryUsesDurableRealtimeSink(t *testing.T) {
	body, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	want := "replyRepository := replycommand.NewGormRepository( resources.DB, replycommand.WithDurableEventSink(realtimeEventSink), )"
	if !strings.Contains(compact, want) {
		t.Fatalf("worker reply repository must persist terminal realtime events with the shared durable sink")
	}
	if want := "DeliveryCommitter: replyDeliveryCommitter{repository: replyRepository}"; !strings.Contains(compact, want) {
		t.Fatalf("worker chat runtime must commit deltas through the shared reply repository")
	}
}

func TestWorkerWiresOneAuthoritativeOfficialModelResolver(t *testing.T) {
	body, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	for _, want := range []string{
		"aiOfficialModelResolver := officialmodel.NewService(officialmodel.NewGormRepository(resources.DB))",
		"PricingResolver: aiOfficialModelResolver",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("worker AI pricing composition missing %q", want)
		}
	}
	if strings.Count(compact, "officialmodel.NewService(officialmodel.NewGormRepository(resources.DB))") != 1 {
		t.Fatal("worker must instantiate exactly one authoritative official model resolver")
	}
}

func TestWorkerUsesRuntimeChatConstructorWithDefaultToolRuntime(t *testing.T) {
	body, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	for _, want := range []string{
		"aiToolRepository := aitool.NewGormRepository(resources.DB)",
		"aitool.DefaultExecutors(aiToolRepository)",
		"aiChatService, err := aichat.NewRuntimeService(aichat.Dependencies{",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("worker AI tool composition missing %q", want)
		}
	}
}

func TestWorkerWiresTrustedCOSStreamingIntoChatRuntime(t *testing.T) {
	body, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	for _, want := range []string{
		"uploadTokenRepository := uploadtoken.NewGormRepository(resources.DB)",
		"aiChatObjectConfig := uploadtoken.NewObjectConfigProvider(uploadTokenRepository, providers.Secretbox)",
		"aiChatObjectStreamer := storagecos.NewObjectStreamer( aiChatObjectConfig,",
		"FileOpener: aiChatObjectStreamer",
	} {
		if !strings.Contains(compact, want) {
			t.Fatalf("worker native-file composition missing %q", want)
		}
	}
	if strings.Count(compact, "uploadtoken.NewObjectConfigProvider(uploadTokenRepository, providers.Secretbox)") != 1 {
		t.Fatal("worker must construct one active COS config source for native-file streaming")
	}
}

func TestWorkerBuildsPaymentWithItsSharedWalletParticipant(t *testing.T) {
	body, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	if !strings.Contains(compact, "walletRepository := walletmodule.NewGormRepository(resources.DB)") || !strings.Contains(compact, "paymentmodule.NewGormRepository(resources.DB, walletRepository)") {
		t.Fatal("worker payment composition must inject the shared wallet participant")
	}
}

func TestWorkerDoesNotConsumeMailDiagnosticWriteOrRekeyCapabilities(t *testing.T) {
	body, err := os.ReadFile("worker.go")
	if err != nil {
		t.Fatalf("read worker composition: %v", err)
	}
	for _, forbidden := range []string{
		"mail.NewService",
		"CreateVerificationLog",
		"NewDiagnosticRekeyService",
		"RewriteDiagnosticCipherBatch",
		"mail_log_verification_codes",
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("worker composition consumes mail diagnostic write capability %q", forbidden)
		}
	}
}

func TestRealtimePublisherForWorkerUsesCodeOwnedDefaultChannel(t *testing.T) {
	publisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{Enabled: true, Publisher: config.RealtimePublisherRedis},
	}, &Resources{})
	if _, ok := publisher.(*infrarealtime.RedisPublisher); !ok {
		t.Fatalf("expected worker redis publisher, got %T", publisher)
	}
	value := reflect.ValueOf(publisher)
	field := value.Elem().FieldByName("channel")
	if !field.IsValid() || field.String() != config.DefaultRealtimeRedisChannel {
		t.Fatalf("expected default channel %q, got %q", config.DefaultRealtimeRedisChannel, field.String())
	}
}

func TestRealtimePublisherForWorkerDoesNotFakeLocalDelivery(t *testing.T) {
	publisher := realtimePublisherForWorker(config.Config{
		Realtime: config.RealtimeConfig{Enabled: true, Publisher: config.RealtimePublisherLocal},
	}, &Resources{})
	if _, ok := publisher.(infrarealtime.NoopPublisher); !ok {
		t.Fatalf("expected worker local mode to stay noop, got %T", publisher)
	}
}

func TestWorkerSchedulerReconcilerHealthIsPartOfReadiness(t *testing.T) {
	base := NewReport(map[string]Check{
		"database":  {Status: StatusUp},
		"scheduler": {Status: StatusUp},
	})

	unhealthy := mergeSchedulerReconcilerHealth(base, true, true, crontask.ReconcileHealth{
		Healthy: false,
		Err:     "cron task schedule is not registered: unknown_task",
	})
	if unhealthy.Status != StatusNotReady || unhealthy.Checks["scheduler"].Status != StatusDown {
		t.Fatalf("unhealthy reconciler must fail worker readiness: %#v", unhealthy)
	}
	if unhealthy.Checks["scheduler"].Message == "" {
		t.Fatalf("unhealthy scheduler must expose the reconciliation error: %#v", unhealthy)
	}

	healthy := mergeSchedulerReconcilerHealth(base, true, true, crontask.ReconcileHealth{
		Healthy:     true,
		LastSuccess: time.Now(),
	})
	if healthy.Status != StatusReady || healthy.Checks["scheduler"].Status != StatusUp {
		t.Fatalf("healthy reconciler must preserve scheduler readiness: %#v", healthy)
	}

	notStarted := mergeSchedulerReconcilerHealth(base, true, false, crontask.ReconcileHealth{})
	if notStarted.Status != StatusNotReady || notStarted.Checks["scheduler"].Status != StatusDown {
		t.Fatalf("enabled scheduler without reconciler must be unready: %#v", notStarted)
	}

	disabled := mergeSchedulerReconcilerHealth(base, false, false, crontask.ReconcileHealth{})
	if disabled.Status != base.Status || disabled.Checks["scheduler"] != base.Checks["scheduler"] {
		t.Fatalf("disabled scheduler must not change readiness: %#v", disabled)
	}
}
