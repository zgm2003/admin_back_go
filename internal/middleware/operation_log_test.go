package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"admin_back_go/internal/telemetry"

	"github.com/gin-gonic/gin"
)

func TestOperationLogSkipsRoutesWithoutOperationRule(t *testing.T) {
	called := false
	router := newOperationLogTestRouter(OperationLogConfig{
		Recorder: func(ctx context.Context, input OperationInput) error {
			called = true
			return nil
		},
	}, nil)
	router.POST("/api/admin/v1/no-log", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/no-log", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("expected route to continue, got %d %s", recorder.Code, recorder.Body.String())
	}
	if called {
		t.Fatalf("expected recorder not to be called for route without operation rule")
	}
}

func TestOperationLogRecordsMatchedRouteAfterHandler(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodPost, "/api/admin/v1/permissions"): {Module: "permission", Action: "create", Title: "新增菜单"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.POST("/api/admin/v1/permissions", func(c *gin.Context) { c.String(http.StatusCreated, "created") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/permissions", nil)
	request.Header.Set(HeaderRequestID, "rid-operation")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated || recorder.Body.String() != "created" {
		t.Fatalf("expected route response to stay intact, got %d %s", recorder.Code, recorder.Body.String())
	}
	if got.UserID != 12 || got.SessionID != 34 || got.Platform != "admin" {
		t.Fatalf("unexpected identity fields: %#v", got)
	}
	if got.Module != "permission" || got.Action != "create" || got.Title != "新增菜单" {
		t.Fatalf("unexpected operation rule fields: %#v", got)
	}
	if got.Method != http.MethodPost || got.Path != "/api/admin/v1/permissions" || got.Status != http.StatusCreated || !got.Success {
		t.Fatalf("unexpected request/status fields: %#v", got)
	}
	if got.RequestID != "rid-operation" {
		t.Fatalf("expected request id rid-operation, got %q", got.RequestID)
	}
}

func TestOperationLogMatchesGinFullPathForRouteParams(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions/:id"): {Module: "permission", Action: "delete", Title: "删除菜单"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.DELETE("/api/admin/v1/permissions/:id", func(c *gin.Context) { c.String(http.StatusOK, "deleted") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/permissions/9", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected route to continue, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got.Action != "delete" || got.Path != "/api/admin/v1/permissions/:id" {
		t.Fatalf("unexpected operation input: %#v", got)
	}
}

func TestOperationLogDoesNotBreakResponseWhenRecorderFails(t *testing.T) {
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodDelete, "/api/admin/v1/permissions/1"): {Module: "permission", Action: "delete", Title: "删除菜单"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			return errors.New("insert log failed")
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.DELETE("/api/admin/v1/permissions/1", func(c *gin.Context) { c.String(http.StatusOK, "deleted") })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodDelete, "/api/admin/v1/permissions/1", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.String() != "deleted" {
		t.Fatalf("expected operation log failure not to alter response, got %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestOperationLogRecordsFailedConfiguredRouteWithStatusAndSuccess(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodPut, "/api/admin/v1/users/:id"): {Module: "user", Action: "update", Title: "编辑用户"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.PUT("/api/admin/v1/users/:id", func(c *gin.Context) { c.JSON(http.StatusBadRequest, gin.H{"code": 100, "msg": "参数错误"}) })

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/admin/v1/users/9", nil)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected bad request response, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if got.Status != http.StatusBadRequest || got.Success {
		t.Fatalf("failed configured route should be logged with status/success=false: %#v", got)
	}
	if got.Path != "/api/admin/v1/users/:id" || got.Module != "user" || got.Action != "update" {
		t.Fatalf("route metadata mismatch: %#v", got)
	}
}

func TestOperationLogCapturesRequestAndResponsePayloadWithoutBreakingHandler(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodPost, "/api/admin/v1/notification-tasks"): {Module: "notification_task", Action: "create", Title: "发布通知任务"},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "admin"})
	router.POST("/api/admin/v1/notification-tasks", func(c *gin.Context) {
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 100, "msg": "bad"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"id": 99, "queued": true}, "msg": "ok"})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/admin/v1/notification-tasks", strings.NewReader(`{"title":"hello","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected route response to stay intact, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	requestPayload, ok := got.RequestPayload.(map[string]any)
	if !ok || requestPayload["title"] != "hello" || requestPayload["password"] != "secret" {
		t.Fatalf("expected decoded request payload before recorder sanitization, got %#v", got.RequestPayload)
	}
	responsePayload, ok := got.ResponsePayload.(map[string]any)
	if !ok || responsePayload["code"] != float64(0) {
		t.Fatalf("expected decoded response payload, got %#v", got.ResponsePayload)
	}
}

func TestOperationLogCanSkipConfiguredPayloads(t *testing.T) {
	var got OperationInput
	router := newOperationLogTestRouter(OperationLogConfig{
		Rules: map[RouteKey]OperationRule{
			NewRouteKey(http.MethodPost, "/api/canvas/v1/ai/images/generations"): {
				Module:              "ai_image",
				Action:              "create_task",
				Title:               "提交Canvas图片任务",
				SkipRequestPayload:  true,
				SkipResponsePayload: true,
			},
		},
		Recorder: func(ctx context.Context, input OperationInput) error {
			got = input
			return nil
		},
	}, &AuthIdentity{UserID: 12, SessionID: 34, Platform: "canvas"})
	router.POST("/api/canvas/v1/ai/images/generations", func(c *gin.Context) {
		var payload map[string]any
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 100, "msg": "bad"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"prompt": payload["prompt"]}, "msg": "ok"})
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/canvas/v1/ai/images/generations", strings.NewReader(`{"prompt":"secret prompt"}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "secret prompt") {
		t.Fatalf("skip payload must not break handler response, got %d %s", recorder.Code, recorder.Body.String())
	}
	if got.RequestPayload != nil || got.ResponsePayload != nil {
		t.Fatalf("expected payload capture to be skipped, got request=%#v response=%#v", got.RequestPayload, got.ResponsePayload)
	}
	if got.Module != "ai_image" || got.Action != "create_task" || got.Status != http.StatusOK || !got.Success {
		t.Fatalf("expected metadata to remain logged, got %#v", got)
	}
}

func TestOperationLogRequired(t *testing.T) {
	t.Run("releases staged response only after audit success", func(t *testing.T) {
		var client *httptest.ResponseRecorder
		var got OperationInput
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(ctx context.Context, input OperationInput) error {
				got = input
				if client.Body.Len() != 0 || client.Header().Get("X-Diagnostic") != "" {
					t.Fatalf("response escaped before audit: headers=%v body=%q", client.Header(), client.Body.String())
				}
				return nil
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			c.Header("X-Diagnostic", "staged")
			c.String(http.StatusCreated, "diagnostic-response")
		})

		client = httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, requiredOperationPath, nil)
		request.Header.Set(HeaderRequestID, "required-success")
		router.ServeHTTP(client, request)

		if client.Code != http.StatusCreated || client.Header().Get("X-Diagnostic") != "staged" || client.Body.String() != "diagnostic-response" {
			t.Fatalf("staged response was not released intact: code=%d headers=%v body=%q", client.Code, client.Header(), client.Body.String())
		}
		if got.UserID != 41 || got.SessionID != 51 || got.RequestID != "required-success" || got.Status != http.StatusCreated || !got.Success {
			t.Fatalf("unexpected required audit input: %#v", got)
		}
		if got.RequestPayload != nil || got.ResponsePayload != nil {
			t.Fatalf("required diagnostic audit captured payload: %#v", got)
		}
	})

	t.Run("committed headers ignore later mutations", func(t *testing.T) {
		tests := []struct {
			name         string
			commit       func(gin.ResponseWriter)
			expectedBody string
		}{
			{
				name: "Write",
				commit: func(writer gin.ResponseWriter) {
					_, _ = writer.Write([]byte("write-body"))
				},
				expectedBody: "write-body",
			},
			{
				name: "WriteString",
				commit: func(writer gin.ResponseWriter) {
					_, _ = writer.WriteString("string-body")
				},
				expectedBody: "string-body",
			},
			{
				name:   "Flush",
				commit: func(writer gin.ResponseWriter) { writer.Flush() },
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				router := newOperationLogTestRouter(OperationLogConfig{
					Rules:    map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
					Recorder: func(context.Context, OperationInput) error { return nil },
				}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
				router.GET(requiredOperationPath, func(c *gin.Context) {
					c.Header("X-Commit-State", "before")
					tt.commit(c.Writer)
					c.Writer.Header().Set("X-Commit-State", "after")
				})

				client := httptest.NewRecorder()
				router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

				if client.Header().Get("X-Commit-State") != "before" || client.Body.String() != tt.expectedBody {
					t.Fatalf("committed response changed: headers=%v body=%q", client.Header(), client.Body.String())
				}
			})
		}
	})

	t.Run("retained context stays staged while recorder blocks", func(t *testing.T) {
		var retained *gin.Context
		recorderEntered := make(chan OperationInput, 1)
		releaseRecorder := make(chan struct{})
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(_ context.Context, input OperationInput) error {
				recorderEntered <- input
				<-releaseRecorder
				return nil
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			retained = c
			c.Header("X-Initial-Stage", "yes")
			_, _ = c.Writer.WriteString("initial")
		})

		client := httptest.NewRecorder()
		served := make(chan struct{})
		go func() {
			router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))
			close(served)
		}()

		var input OperationInput
		select {
		case input = <-recorderEntered:
		case <-time.After(time.Second):
			t.Fatal("required recorder was not reached")
		}
		_, stayedStaged := retained.Writer.(*requiredAuditWriter)
		retained.Writer.Header().Set("X-Late-Stage", "yes")
		written, writeErr := retained.Writer.WriteString("-late")
		escapedBody := client.Body.String()
		escapedInitialHeader := client.Header().Get("X-Initial-Stage")
		escapedLateHeader := client.Header().Get("X-Late-Stage")
		close(releaseRecorder)
		select {
		case <-served:
		case <-time.After(time.Second):
			t.Fatal("required response was not released")
		}

		if !stayedStaged || escapedBody != "" || escapedInitialHeader != "" || escapedLateHeader != "" {
			t.Fatalf("retained context bypassed audit gate: staged=%v body=%q initial_header=%q late_header=%q", stayedStaged, escapedBody, escapedInitialHeader, escapedLateHeader)
		}
		if writeErr == nil || written != 0 {
			t.Fatalf("late sealed write=(%d, %v)", written, writeErr)
		}
		if input.RequestPayload != nil || input.ResponsePayload != nil {
			t.Fatalf("required audit captured payload: %#v", input)
		}
		if client.Code != http.StatusOK || client.Body.String() != "initial" || client.Header().Get("X-Initial-Stage") != "yes" || client.Header().Get("X-Late-Stage") != "" {
			t.Fatalf("bounded staged response was not released after audit: code=%d headers=%v body=%q", client.Code, client.Header(), client.Body.String())
		}
		if _, stillStaged := retained.Writer.(*requiredAuditWriter); stillStaged {
			t.Fatalf("writer was not restored after response release")
		}
	})

	t.Run("late recorder write cannot cross staging cap", func(t *testing.T) {
		var retained *gin.Context
		var client *httptest.ResponseRecorder
		var got OperationInput
		var lateWriteErr error
		lateWritten := -1
		recorderCalls := 0
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(_ context.Context, input OperationInput) error {
				recorderCalls++
				got = input
				if client.Body.Len() != 0 {
					t.Fatalf("full staged response escaped before audit: size=%d", client.Body.Len())
				}
				lateWritten, lateWriteErr = retained.Writer.WriteString("x")
				return nil
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			retained = c
			_, _ = c.Writer.Write(bytes.Repeat([]byte("a"), 1<<20))
		})

		client = httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		if recorderCalls != 1 {
			t.Fatalf("recorder calls=%d, want 1", recorderCalls)
		}
		if got.Status != http.StatusOK || !got.Success || got.RequestPayload != nil || got.ResponsePayload != nil {
			t.Fatalf("late-cap audit input=%#v", got)
		}
		if lateWritten != 0 || lateWriteErr == nil {
			t.Fatalf("late cap write=(%d, %v)", lateWritten, lateWriteErr)
		}
		if client.Code != http.StatusOK || client.Body.Len() != 1<<20 {
			t.Fatalf("frozen response code=%d size=%d", client.Code, client.Body.Len())
		}
	})

	t.Run("recorder failure discards staged output", func(t *testing.T) {
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(context.Context, OperationInput) error {
				return errors.New("audit unavailable")
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			c.Header("X-Diagnostic", "must-not-escape")
			c.String(http.StatusOK, "diagnostic-response")
		})

		client := httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		assertRequiredAuditFailureResponse(t, client, "diagnostic-response")
		if client.Header().Get("X-Diagnostic") != "" {
			t.Fatalf("staged header escaped after audit failure: %v", client.Header())
		}
	})

	t.Run("nil recorder fails before handler", func(t *testing.T) {
		handlerCalled := false
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			handlerCalled = true
			c.String(http.StatusOK, "diagnostic-response")
		})

		client := httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		assertRequiredAuditFailureResponse(t, client, "diagnostic-response")
		if handlerCalled {
			t.Fatalf("required route handler ran without an audit recorder")
		}
	})

	t.Run("exact one MiB response succeeds", func(t *testing.T) {
		body := bytes.Repeat([]byte("a"), 1<<20)
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules:    map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(context.Context, OperationInput) error { return nil },
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			_, _ = c.Writer.Write(body)
		})

		client := httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))
		if client.Code != http.StatusOK || client.Body.Len() != len(body) || !bytes.Equal(client.Body.Bytes(), body) {
			t.Fatalf("exact limit response failed: code=%d size=%d", client.Code, client.Body.Len())
		}
	})

	t.Run("overflow records failed subject audit and returns fixed error", func(t *testing.T) {
		var got OperationInput
		body := bytes.Repeat([]byte("b"), (1<<20)+1)
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(ctx context.Context, input OperationInput) error {
				got = input
				return nil
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			_, _ = c.Writer.Write(body)
		})

		client := httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		assertRequiredAuditFailureResponse(t, client, strings.Repeat("b", 32))
		if got.UserID != 41 || got.SessionID != 51 || got.Status != http.StatusInternalServerError || got.Success {
			t.Fatalf("overflow audit was not a failed subject-bearing record: %#v", got)
		}
		if got.RequestPayload != nil || got.ResponsePayload != nil {
			t.Fatalf("overflow audit captured payload: %#v", got)
		}
	})

	t.Run("handler error is audited before release", func(t *testing.T) {
		var client *httptest.ResponseRecorder
		var got OperationInput
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(ctx context.Context, input OperationInput) error {
				got = input
				if client.Body.Len() != 0 {
					t.Fatalf("handler error escaped before audit: %q", client.Body.String())
				}
				return nil
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			c.JSON(http.StatusBadRequest, gin.H{"code": 100, "detail": "diagnostic-error"})
		})

		client = httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		if client.Code != http.StatusBadRequest || !strings.Contains(client.Body.String(), "diagnostic-error") {
			t.Fatalf("handler error was not released intact: code=%d body=%q", client.Code, client.Body.String())
		}
		if got.Status != http.StatusBadRequest || got.Success || got.ResponsePayload != nil {
			t.Fatalf("handler error audit mismatch: %#v", got)
		}
	})

	t.Run("handler panic is audited and fails closed", func(t *testing.T) {
		tests := []struct {
			name        string
			recorderErr error
		}{
			{name: "recorder success"},
			{name: "recorder failure", recorderErr: errors.New("audit unavailable")},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				const (
					stagedMarker  = "panic-staged-marker"
					requestMarker = "panic-request-marker"
					panicMarker   = "panic-value-marker"
					outerMarker   = "outer-recovery-marker"
				)
				var got OperationInput
				var retained *gin.Context
				var client *httptest.ResponseRecorder
				var logBuffer bytes.Buffer
				recorderCalls := 0
				rule := requiredOperationRule()
				rule.SkipRequestPayload = false
				rule.SkipResponsePayload = false
				router := newOperationLogPanicTestRouter(OperationLogConfig{
					Rules: map[RouteKey]OperationRule{
						NewRouteKey(http.MethodPost, requiredOperationPath): rule,
					},
					Recorder: func(_ context.Context, input OperationInput) error {
						recorderCalls++
						got = input
						if client.Body.Len() != 0 || client.Header().Get("X-Panic-Stage") != "" {
							t.Fatalf("panic response escaped before audit: headers=%v body=%q", client.Header(), client.Body.String())
						}
						if _, staged := retained.Writer.(*requiredAuditWriter); !staged {
							t.Fatalf("panic recorder did not retain staging writer")
						}
						return tt.recorderErr
					},
					Logger: slog.New(slog.NewJSONHandler(&logBuffer, nil)),
				}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"}, outerMarker)
				router.POST(requiredOperationPath, func(c *gin.Context) {
					retained = c
					c.Header("X-Panic-Stage", "hidden")
					_, _ = c.Writer.WriteString(stagedMarker)
					panic(panicMarker)
				})

				client = httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, requiredOperationPath, strings.NewReader(`{"field":"`+requestMarker+`"}`))
				request.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(client, request)

				if recorderCalls != 1 {
					t.Fatalf("panic recorder calls=%d, want 1", recorderCalls)
				}
				if got.Status != http.StatusInternalServerError || got.Success || got.RequestPayload != nil || got.ResponsePayload != nil {
					t.Fatalf("panic audit input=%#v", got)
				}
				assertRequiredAuditFailureResponse(t, client, stagedMarker)
				if client.Header().Get("X-Panic-Stage") != "" {
					t.Fatalf("panic staged header escaped: %v", client.Header())
				}
				for _, forbidden := range []string{requestMarker, panicMarker, outerMarker} {
					if strings.Contains(client.Body.String(), forbidden) || strings.Contains(logBuffer.String(), forbidden) {
						t.Fatalf("panic path exposed forbidden marker")
					}
				}
				if !strings.Contains(logBuffer.String(), "required operation handler panicked") {
					t.Fatalf("missing sanitized panic warning: %s", logBuffer.String())
				}
				if _, stillStaged := retained.Writer.(*requiredAuditWriter); stillStaged {
					t.Fatalf("panic path did not restore destination writer")
				}
			})
		}
	})

	t.Run("recorder panic fails closed on every required path", func(t *testing.T) {
		const (
			stagedMarker        = "recorder-panic-staged-marker"
			recorderPanicMarker = "recorder-panic-sensitive-value"
			handlerPanicMarker  = "handler-panic-sensitive-value"
			outerMarker         = "outer-recorder-panic-recovery"
		)
		tests := []struct {
			name    string
			handler gin.HandlerFunc
		}{
			{
				name: "normal response",
				handler: func(c *gin.Context) {
					c.Header("X-Recorder-Panic", "hidden")
					_, _ = c.Writer.WriteString(stagedMarker)
				},
			},
			{
				name: "response overflow",
				handler: func(c *gin.Context) {
					c.Header("X-Recorder-Panic", "hidden")
					body := append([]byte(stagedMarker), bytes.Repeat([]byte("x"), maxRequiredAuditResponseBytes+1-len(stagedMarker))...)
					_, _ = c.Writer.Write(body)
				},
			},
			{
				name: "handler panic",
				handler: func(c *gin.Context) {
					c.Header("X-Recorder-Panic", "hidden")
					_, _ = c.Writer.WriteString(stagedMarker)
					panic(handlerPanicMarker)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var retained *gin.Context
				var logBuffer bytes.Buffer
				metrics := &rawOperationTelemetry{}
				recorderCalls := 0
				router := newOperationLogPanicTestRouter(OperationLogConfig{
					Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
					Recorder: func(context.Context, OperationInput) error {
						recorderCalls++
						panic(recorderPanicMarker)
					},
					Logger:    slog.New(slog.NewJSONHandler(&logBuffer, nil)),
					Telemetry: metrics,
				}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"}, outerMarker)
				router.GET(requiredOperationPath, func(c *gin.Context) {
					retained = c
					tt.handler(c)
				})

				client := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, requiredOperationPath, nil)
				request.Header.Set(HeaderRequestID, "recorder-panic-request")
				router.ServeHTTP(client, request)

				if recorderCalls != 1 {
					t.Fatalf("recorder calls=%d, want 1", recorderCalls)
				}
				assertRequiredAuditFailureResponse(t, client, stagedMarker)
				if client.Header().Get("X-Recorder-Panic") != "" {
					t.Fatalf("staged header escaped recorder panic: %v", client.Header())
				}
				if retained == nil {
					t.Fatal("handler did not retain context")
				}
				if _, stillStaged := retained.Writer.(*requiredAuditWriter); stillStaged {
					t.Fatal("recorder panic did not restore destination writer")
				}
				for _, forbidden := range []string{stagedMarker, recorderPanicMarker, handlerPanicMarker, outerMarker} {
					if strings.Contains(client.Body.String(), forbidden) || strings.Contains(logBuffer.String(), forbidden) {
						t.Fatalf("recorder panic path exposed forbidden marker %q", forbidden)
					}
				}
				if !strings.Contains(logBuffer.String(), "required operation audit failed") ||
					!strings.Contains(logBuffer.String(), requiredAuditRecorderFailed) {
					t.Fatalf("missing sanitized recorder failure warning: %s", logBuffer.String())
				}
				assertRequiredAuditFailureTelemetry(t, metrics, requiredAuditRecorderFailed)
			})
		}
	})

	t.Run("stages headers status body and flush", func(t *testing.T) {
		var client *httptest.ResponseRecorder
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(context.Context, OperationInput) error {
				if client.Body.Len() != 0 || client.Flushed || client.Header().Get("X-Stage") != "" {
					t.Fatalf("staged writer forwarded output before audit: headers=%v flushed=%v body=%q", client.Header(), client.Flushed, client.Body.String())
				}
				return nil
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			c.Header("X-Stage", "yes")
			c.Status(http.StatusAccepted)
			_, _ = c.Writer.WriteString("first")
			c.Writer.Flush()
			_, _ = c.Writer.Write([]byte("-second"))
		})

		client = httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		if client.Code != http.StatusAccepted || client.Header().Get("X-Stage") != "yes" || client.Body.String() != "first-second" || !client.Flushed {
			t.Fatalf("staged response release mismatch: code=%d headers=%v flushed=%v body=%q", client.Code, client.Header(), client.Flushed, client.Body.String())
		}
	})

	t.Run("rejects response escape through optional interfaces", func(t *testing.T) {
		var hijackErr error
		var exposesReaderFrom bool
		var exposesUnwrap bool
		var exposesRelease bool
		var exposesPusher bool
		var returnsPusher bool
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules:    map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(context.Context, OperationInput) error { return nil },
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			_, _, hijackErr = c.Writer.Hijack()
			_, exposesReaderFrom = any(c.Writer).(io.ReaderFrom)
			_, exposesUnwrap = any(c.Writer).(interface{ Unwrap() http.ResponseWriter })
			_, exposesRelease = any(c.Writer).(interface{ Release() error })
			_, exposesPusher = any(c.Writer).(http.Pusher)
			returnsPusher = c.Writer.Pusher() != nil
			c.String(http.StatusOK, "safe")
		})

		client := httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		if hijackErr == nil {
			t.Fatalf("required staging writer allowed Hijack")
		}
		if exposesReaderFrom || exposesUnwrap || exposesRelease || exposesPusher || returnsPusher {
			t.Fatalf("required staging writer exposed output interface: readerFrom=%v unwrap=%v release=%v pusher=%v returnedPusher=%v", exposesReaderFrom, exposesUnwrap, exposesRelease, exposesPusher, returnsPusher)
		}
		if client.Code != http.StatusOK || client.Body.String() != "safe" {
			t.Fatalf("safe response behavior changed: code=%d body=%q", client.Code, client.Body.String())
		}
	})

	t.Run("all fail-closed paths warn and count without payload", func(t *testing.T) {
		tests := []struct {
			name     string
			reason   string
			recorder OperationRecorder
			handler  gin.HandlerFunc
		}{
			{
				name:   "recorder failure",
				reason: requiredAuditRecorderFailed,
				recorder: func(context.Context, OperationInput) error {
					return errors.New("audit unavailable")
				},
				handler: func(c *gin.Context) { c.String(http.StatusOK, "private-diagnostic-payload") },
			},
			{
				name:   "missing recorder",
				reason: requiredAuditRecorderMissing,
				handler: func(c *gin.Context) {
					c.String(http.StatusOK, "private-diagnostic-payload")
				},
			},
			{
				name:     "overflow",
				reason:   requiredAuditResponseOverflowed,
				recorder: func(context.Context, OperationInput) error { return nil },
				handler:  func(c *gin.Context) { _, _ = c.Writer.Write(bytes.Repeat([]byte("p"), (1<<20)+1)) },
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				var logBuffer bytes.Buffer
				logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))
				metrics := &rawOperationTelemetry{}
				router := newOperationLogTestRouter(OperationLogConfig{
					Rules:     map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
					Recorder:  tt.recorder,
					Logger:    logger,
					Telemetry: metrics,
				}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
				router.GET(requiredOperationPath, tt.handler)

				client := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodGet, requiredOperationPath, nil)
				request.Header.Set(HeaderRequestID, "required-failure")
				router.ServeHTTP(client, request)

				assertRequiredAuditFailureResponse(t, client, "private-diagnostic-payload")
				if !strings.Contains(logBuffer.String(), "required operation audit failed") ||
					!strings.Contains(logBuffer.String(), "required-failure") ||
					!strings.Contains(logBuffer.String(), requiredOperationPath) {
					t.Fatalf("missing structured required-audit warning: %s", logBuffer.String())
				}
				if strings.Contains(logBuffer.String(), "private-diagnostic-payload") {
					t.Fatalf("warning leaked payload: %s", logBuffer.String())
				}
				assertRequiredAuditFailureTelemetry(t, metrics, tt.reason)
				attributes := metrics.counts[0].attributes
				for _, forbiddenKey := range []string{"user_id", "session_id", "request_id"} {
					if _, exists := attributes[forbiddenKey]; exists {
						t.Fatalf("telemetry contains identity key %q: %+v", forbiddenKey, attributes)
					}
				}
				if len(attributes) != 5 {
					t.Fatalf("telemetry attributes=%+v, want exactly five low-cardinality keys", attributes)
				}
				encoded, err := json.Marshal(attributes)
				if err != nil {
					t.Fatalf("encode telemetry attributes: %v", err)
				}
				if strings.Contains(string(encoded), "private-diagnostic-payload") {
					t.Fatalf("telemetry leaked payload: %s", encoded)
				}
			})
		}
	})

	t.Run("ordinary rule remains fail open", func(t *testing.T) {
		rule := requiredOperationRule()
		rule.Required = false
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): rule},
			Recorder: func(context.Context, OperationInput) error {
				return errors.New("audit unavailable")
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) { c.String(http.StatusOK, "ordinary-response") })

		client := httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))
		if client.Code != http.StatusOK || client.Body.String() != "ordinary-response" {
			t.Fatalf("ordinary audit stopped failing open: code=%d body=%q", client.Code, client.Body.String())
		}
	})
}

func TestOperationLogRequiredDetectsNilPanicCompatibility(t *testing.T) {
	const childEnvironment = "ADMIN_BACK_GO_PANICNIL_CHILD"
	if os.Getenv(childEnvironment) == "1" {
		var got OperationInput
		recorderCalls := 0
		router := newOperationLogTestRouter(OperationLogConfig{
			Rules: map[RouteKey]OperationRule{requiredOperationRouteKey(): requiredOperationRule()},
			Recorder: func(_ context.Context, input OperationInput) error {
				recorderCalls++
				got = input
				return nil
			},
		}, &AuthIdentity{UserID: 41, SessionID: 51, Platform: "admin"})
		router.GET(requiredOperationPath, func(c *gin.Context) {
			c.Header("X-Panic-Nil", "hidden")
			_, _ = c.Writer.WriteString("panic-nil-staged-marker")
			panic(nil)
		})

		client := httptest.NewRecorder()
		router.ServeHTTP(client, httptest.NewRequest(http.MethodGet, requiredOperationPath, nil))

		if recorderCalls != 1 || got.Status != http.StatusInternalServerError || got.Success {
			t.Fatalf("panic(nil) audit calls=%d input=%#v", recorderCalls, got)
		}
		assertRequiredAuditFailureResponse(t, client, "panic-nil-staged-marker")
		if client.Header().Get("X-Panic-Nil") != "" {
			t.Fatalf("panic(nil) staged header escaped: %v", client.Header())
		}
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestOperationLogRequiredDetectsNilPanicCompatibility$")
	command.Env = append(os.Environ(), childEnvironment+"=1", "GODEBUG=panicnil=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("panicnil compatibility subprocess failed: %v\n%s", err, output)
	}
}

const requiredOperationPath = "/api/admin/v1/mail/logs"

func requiredOperationRouteKey() RouteKey {
	return NewRouteKey(http.MethodGet, requiredOperationPath)
}

func requiredOperationRule() OperationRule {
	return OperationRule{
		Module: "mail", Action: "list_logs", Title: "查看邮件日志及验证码",
		SkipRequestPayload: true, SkipResponsePayload: true, Required: true,
	}
}

func assertRequiredAuditFailureResponse(t *testing.T, recorder *httptest.ResponseRecorder, forbidden string) {
	t.Helper()
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body_size=%d", recorder.Code, recorder.Body.Len())
	}
	if recorder.Header().Get("Cache-Control") != "no-store, private" || recorder.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("required audit failure is cacheable: %v", recorder.Header())
	}
	if forbidden != "" && strings.Contains(recorder.Body.String(), forbidden) {
		t.Fatalf("required audit failure leaked staged output")
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("required audit failure content type=%q", contentType)
	}
	var body struct {
		Code  int            `json:"code"`
		Data  map[string]any `json:"data"`
		Msg   string         `json:"msg"`
		Error struct {
			Code      string `json:"code"`
			Category  string `json:"category"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode required audit failure JSON: %v", err)
	}
	if body.Code != http.StatusInternalServerError || len(body.Data) != 0 || body.Msg != "系统错误" ||
		body.Error.Code != "internal.unknown" || body.Error.Category != "internal" || body.Error.Retryable {
		t.Fatalf("required audit failure body=%+v", body)
	}
}

func assertRequiredAuditFailureTelemetry(t *testing.T, metrics *rawOperationTelemetry, reason string) {
	t.Helper()
	if len(metrics.counts) != 1 {
		t.Fatalf("telemetry counts=%+v", metrics.counts)
	}
	count := metrics.counts[0]
	if count.name != requiredAuditFailureMetric || count.delta != 1 ||
		count.attributes["http.method"] != http.MethodGet ||
		count.attributes["http.route"] != requiredOperationPath ||
		count.attributes["http.status"] != http.StatusInternalServerError ||
		count.attributes["error.code"] != reason || count.attributes["outcome"] != "error" {
		t.Fatalf("required-audit telemetry mismatch: %+v", count)
	}
}

type operationTelemetryCount struct {
	name       string
	delta      float64
	attributes telemetry.Attributes
}

type rawOperationTelemetry struct {
	counts []operationTelemetryCount
}

func (r *rawOperationTelemetry) Count(name string, delta float64, attributes telemetry.Attributes) {
	cloned := make(telemetry.Attributes, len(attributes))
	for key, value := range attributes {
		cloned[key] = value
	}
	r.counts = append(r.counts, operationTelemetryCount{name: name, delta: delta, attributes: cloned})
}

func (*rawOperationTelemetry) Observe(string, float64, telemetry.Attributes) {}

func (*rawOperationTelemetry) Start(ctx context.Context, _ string, _ telemetry.Attributes) (context.Context, func(error)) {
	return ctx, func(error) {}
}

func newOperationLogTestRouter(cfg OperationLogConfig, identity *AuthIdentity) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(ContextAuthIdentity, identity)
			c.Next()
		})
	}
	router.Use(OperationLog(cfg))
	return router
}

func newOperationLogPanicTestRouter(cfg OperationLogConfig, identity *AuthIdentity, outerMarker string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(RequestID())
	router.Use(func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				c.String(http.StatusInternalServerError, outerMarker)
			}
		}()
		c.Next()
	})
	if identity != nil {
		router.Use(func(c *gin.Context) {
			c.Set(ContextAuthIdentity, identity)
			c.Next()
		})
	}
	router.Use(OperationLog(cfg))
	return router
}
