package runtime

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/server/adminroute"
)

func TestAPIThreadsMailDiagnosticBoxIntoAdminProviders(t *testing.T) {
	body, err := os.ReadFile("api.go")
	if err != nil {
		t.Fatalf("read API composition: %v", err)
	}
	compact := strings.Join(strings.Fields(string(body)), " ")
	if !strings.Contains(compact, "MailDiagnosticBox: providers.MailDiagnosticBox,") {
		t.Fatal("API composition does not thread the mail diagnostic box into Admin")
	}
}

func TestNewAPIValidatesSecretsWithoutOpeningResources(t *testing.T) {
	runtime, err := NewAPI(config.Config{}, slog.Default(), adminroute.NewRegistry())
	if runtime != nil || err == nil || !strings.Contains(err.Error(), "APP_SECRET") {
		t.Fatalf("runtime=%+v err=%v", runtime, err)
	}

	runtime, err = NewAPI(config.Config{
		App: config.AppConfig{Secret: strings.Repeat("a", 64)},
	}, slog.Default(), adminroute.NewRegistry())
	if err != nil || runtime == nil {
		t.Fatalf("constructor should not open external resources: runtime=%+v err=%v", runtime, err)
	}
}

func TestAPIRuntimeStartsAndStopsInLifecycleOrder(t *testing.T) {
	var mu sync.Mutex
	events := make([]string, 0, 12)
	record := func(event string) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}

	server := newFakeHTTPServer(record)
	hooks := apiHooks{
		openResources: func(context.Context) (func(context.Context) Report, CleanupFunc, error) {
			record("start:resources")
			return func(context.Context) Report {
					return NewReport(map[string]Check{"database": {Status: StatusUp}})
				}, func(context.Context) error {
					record("stop:resources")
					return nil
				}, nil
		},
		buildProviders: lifecycleHook(record, "providers", "queue_producer"),
		buildAdmin:     lifecycleHook(record, "admin_graph", ""),
		buildRouter:    lifecycleHook(record, "router", ""),
		startRealtime:  lifecycleHook(record, "realtime", "realtime"),
		openHTTP: func(context.Context) (HTTPServer, error) {
			record("start:http")
			return server, nil
		},
	}
	runtime := newAPIRuntimeWithHooks(hooks)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() { started <- runtime.Start(ctx) }()
	server.waitListening(t)
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
		"start:admin_graph",
		"start:router",
		"start:realtime",
		"start:http",
		"listen:http",
		"stop:http",
		"stop:realtime",
		"stop:queue_producer",
		"stop:resources",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v\nwant=%v", got, want)
	}
}

func lifecycleHook(record func(string), start string, stop string) func(context.Context) (CleanupFunc, error) {
	return func(context.Context) (CleanupFunc, error) {
		record("start:" + start)
		if stop == "" {
			return nil, nil
		}
		return func(context.Context) error {
			record("stop:" + stop)
			return nil
		}, nil
	}
}

type fakeHTTPServer struct {
	record    func(string)
	listening chan struct{}
	stopped   chan struct{}
	stopOnce  sync.Once
}

func newFakeHTTPServer(record func(string)) *fakeHTTPServer {
	return &fakeHTTPServer{
		record:    record,
		listening: make(chan struct{}),
		stopped:   make(chan struct{}),
	}
}

func (s *fakeHTTPServer) ListenAndServe() error {
	s.record("listen:http")
	close(s.listening)
	<-s.stopped
	return http.ErrServerClosed
}

func (s *fakeHTTPServer) Shutdown(context.Context) error {
	s.stopOnce.Do(func() {
		s.record("stop:http")
		close(s.stopped)
	})
	return nil
}

func (s *fakeHTTPServer) waitListening(t *testing.T) {
	t.Helper()
	select {
	case <-s.listening:
	case <-t.Context().Done():
		t.Fatal("HTTP server did not start")
	}
}

var _ HTTPServer = (*fakeHTTPServer)(nil)

func TestAPIRuntimeRejectsConcurrentSecondStart(t *testing.T) {
	server := newFakeHTTPServer(func(string) {})
	runtime := newAPIRuntimeWithHooks(apiHooks{
		openResources: func(context.Context) (func(context.Context) Report, CleanupFunc, error) {
			return nil, nil, nil
		},
		openHTTP: func(context.Context) (HTTPServer, error) { return server, nil },
	})

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	go func() { started <- runtime.Start(ctx) }()
	server.waitListening(t)

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

func TestAPIRuntimeCleansAfterFailureAtEveryBoundary(t *testing.T) {
	boundaries := []string{"resources", "providers", "admin", "router", "realtime", "http", "listen"}
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
			hooks := apiHooks{
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
				buildAdmin:     step("admin"),
				buildRouter:    step("router"),
				startRealtime:  step("realtime"),
				openHTTP: func(context.Context) (HTTPServer, error) {
					if failAt == "http" {
						return nil, failure
					}
					return &failingHTTPServer{failure: failure, stopped: &stopped}, nil
				},
			}
			err := newAPIRuntimeWithHooks(hooks).Start(context.Background())
			if !errors.Is(err, failure) {
				t.Fatalf("Start error=%v, want %v", err, failure)
			}

			startedBeforeFailure := []string{"resources", "providers", "admin", "router", "realtime", "http"}
			failureIndex := 0
			for index, name := range boundaries {
				if name == failAt {
					failureIndex = index
					break
				}
			}
			wantStopped := append([]string(nil), startedBeforeFailure[:min(failureIndex, len(startedBeforeFailure))]...)
			for left, right := 0, len(wantStopped)-1; left < right; left, right = left+1, right-1 {
				wantStopped[left], wantStopped[right] = wantStopped[right], wantStopped[left]
			}
			if !reflect.DeepEqual(stopped, wantStopped) {
				t.Fatalf("stopped=%v want=%v", stopped, wantStopped)
			}
		})
	}
}

func TestAPIRuntimeHealthRemainsStoppedAfterShutdown(t *testing.T) {
	server := newFakeHTTPServer(func(string) {})
	runtime := newAPIRuntimeWithHooks(apiHooks{
		openResources: func(context.Context) (func(context.Context) Report, CleanupFunc, error) {
			return func(context.Context) Report {
				return NewReport(map[string]Check{"database": {Status: StatusUp}})
			}, nil, nil
		},
		openHTTP: func(context.Context) (HTTPServer, error) { return server, nil },
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- runtime.Start(ctx) }()
	server.waitListening(t)
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

func TestAPIRuntimeClosesStepThatFinishesDuringShutdown(t *testing.T) {
	providerStarted := make(chan struct{})
	releaseProvider := make(chan struct{})
	providerClosed := make(chan struct{})
	runtime := newAPIRuntimeWithHooks(apiHooks{
		openResources: func(context.Context) (func(context.Context) Report, CleanupFunc, error) {
			return nil, nil, nil
		},
		buildProviders: func(context.Context) (CleanupFunc, error) {
			close(providerStarted)
			<-releaseProvider
			return func(context.Context) error {
				close(providerClosed)
				return nil
			}, nil
		},
	})
	started := make(chan error, 1)
	go func() { started <- runtime.Start(context.Background()) }()
	<-providerStarted
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	close(releaseProvider)
	if err := <-started; !errors.Is(err, ErrCleanupClosed) {
		t.Fatalf("Start error=%v, want %v", err, ErrCleanupClosed)
	}
	select {
	case <-providerClosed:
	case <-time.After(time.Second):
		t.Fatal("late provider cleanup was not executed")
	}
}

type failingHTTPServer struct {
	failure error
	stopped *[]string
}

func (server *failingHTTPServer) ListenAndServe() error { return server.failure }

func (server *failingHTTPServer) Shutdown(context.Context) error {
	*server.stopped = append(*server.stopped, "http")
	return nil
}
