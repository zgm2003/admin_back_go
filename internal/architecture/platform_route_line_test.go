package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlatformRouteLineOwnership(t *testing.T) {
	root := backendRoot(t)
	for _, rel := range []string{
		"internal/module/auth/transport/canvas/route.go",
		"internal/module/auth/transport/canvas/handler.go",
		"internal/module/user/transport/app/route.go",
		"internal/module/user/transport/app/handler.go",
		"internal/module/user/transport/canvas/route.go",
		"internal/module/user/transport/canvas/handler.go",
		"internal/module/profile/transport/canvas/route.go",
		"internal/module/profile/transport/canvas/handler.go",
		"internal/module/payment/transport/canvas/route.go",
		"internal/module/payment/transport/canvas/handler.go",
		"internal/module/payment/wallet/transport/canvas/route.go",
		"internal/module/payment/wallet/transport/canvas/handler.go",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	routesAuth := readRouteLineSource(t, root, "internal/server/routes_auth.go")
	mustContainRouteLine(t, routesAuth, `authcanvas "admin_back_go/internal/module/auth/transport/canvas"`)
	mustContainRouteLine(t, routesAuth, `authcanvas.Register(router, authcanvas.Dependencies{`)
	mustNotContainRouteLine(t, routesAuth, `Prefix:         "/api/canvas/v1/auth"`)
	mustNotContainRouteLine(t, routesAuth, `authapp.RouteOptions`)

	routesUser := readRouteLineSource(t, root, "internal/server/routes_admin_user.go")
	mustContainRouteLine(t, routesUser, `userapp "admin_back_go/internal/module/user/transport/app"`)
	mustContainRouteLine(t, routesUser, `usercanvas "admin_back_go/internal/module/user/transport/canvas"`)
	mustContainRouteLine(t, routesUser, `profilecanvas "admin_back_go/internal/module/profile/transport/canvas"`)
	mustContainRouteLine(t, routesUser, `users := deps.Admin.Identity.Users`)
	mustContainRouteLine(t, routesUser, `userapp.RegisterRoutes(router, users)`)
	mustContainRouteLine(t, routesUser, `usercanvas.RegisterRoutes(router, users)`)
	mustContainRouteLine(t, routesUser, `profilecanvas.RegisterRoutes(router, users)`)
	mustNotContainRouteLine(t, routesUser, `RegisterRoutesWithOptions`)
	mustNotContainRouteLine(t, routesUser, `UsersPrefix: "/api/canvas/v1/users"`)

	routesCommerce := readRouteLineSource(t, root, "internal/server/routes_admin_commerce_rbac.go")
	mustContainRouteLine(t, routesCommerce, `paymentcanvas "admin_back_go/internal/module/payment/transport/canvas"`)
	mustContainRouteLine(t, routesCommerce, `walletcanvas "admin_back_go/internal/module/payment/wallet/transport/canvas"`)
	mustContainRouteLine(t, routesCommerce, `commerce := deps.Admin.Commerce`)
	mustContainRouteLine(t, routesCommerce, `walletcanvas.RegisterRoutes(router, commerce.Wallet)`)
	mustContainRouteLine(t, routesCommerce, `paymentcanvas.RegisterRechargeRoutes(router, commerce.Payment)`)
	mustNotContainRouteLine(t, routesCommerce, `walletadmin.RegisterCurrentUserRoutes(router, "/api/canvas/v1/wallet"`)
	mustNotContainRouteLine(t, routesCommerce, `paymentadmin.RegisterRechargeRoutes(router, "/api/canvas/v1/payment/recharges"`)
}

func TestPlatformRouteLineNoCrossPlatformURLPrefixInsideWrongTransport(t *testing.T) {
	root := backendRoot(t)
	moduleRoot := filepath.Join(root, "internal", "module")
	var offenders []string
	err := filepath.WalkDir(moduleRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if !strings.Contains(rel, "/transport/") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		switch {
		case strings.Contains(rel, "/transport/app/") && strings.Contains(text, `"/api/canvas/v1`):
			offenders = append(offenders, rel+" contains canvas URL inside app transport")
		case strings.Contains(rel, "/transport/admin/") && strings.Contains(text, `"/api/canvas/v1`):
			offenders = append(offenders, rel+" contains canvas URL inside admin transport")
		case strings.Contains(rel, "/transport/canvas/") && strings.Contains(text, `"/api/app/v1`):
			offenders = append(offenders, rel+" contains app URL inside canvas transport")
		case strings.Contains(rel, "/transport/canvas/") && strings.Contains(text, `"/api/admin/v1`):
			offenders = append(offenders, rel+" contains admin URL inside canvas transport")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk transport files: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("platform URL prefix must stay inside matching transport package:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestCanvasAIImageRoutesOwnedByAIImageTransport(t *testing.T) {
	root := backendRoot(t)

	canvasRoute := readRouteLineSource(t, root, "internal/module/canvas/transport/canvas/route.go")
	mustNotContainRouteLine(t, canvasRoute, `"/ai/images`)

	aiImageCanvasRoute := filepath.Join(root, "internal", "module", "ai", "image", "transport", "canvas", "route.go")
	if _, err := os.Stat(aiImageCanvasRoute); err != nil {
		t.Fatalf("expected ai/image canvas route transport to exist: %v", err)
	}

	routesCanvas := readRouteLineSource(t, root, "internal/server/routes_canvas.go")
	mustContainRouteLine(t, routesCanvas, `aiimagecanvas "admin_back_go/internal/module/ai/image/transport/canvas"`)
	mustContainRouteLine(t, routesCanvas, `retired := deps.Retired`)
	mustContainRouteLine(t, routesCanvas, `aiimagecanvas.RegisterRoutes(router, retired.AIImages)`)
	mustNotContainRouteLine(t, routesCanvas, `canvasimagecanvas`)
}

func TestCanvasModuleProductionCodeDoesNotOwnAIImageRuntime(t *testing.T) {
	root := backendRoot(t)
	mustNotExist(t, root, "internal/module/canvas/image")

	canvasRoot := filepath.Join(root, "internal", "module", "canvas")
	forbidden := []string{"ImageTask) TableName", "ImageFile) TableName", "CreateWithUploadedFiles(", "GenerateImage("}
	var offenders []string

	err := filepath.WalkDir(canvasRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(rel)+" contains "+token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk canvas module: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("canvas module production code must not own AI image runtime:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestCanvasAIChatRoutesOwnedByAIChatTransport(t *testing.T) {
	root := backendRoot(t)
	canvasRoute := readRouteLineSource(t, root, "internal/module/canvas/transport/canvas/route.go")
	mustNotContainRouteLine(t, canvasRoute, `"/ai/chat`)

	aiChatCanvasRoute := filepath.Join(root, "internal", "module", "ai", "chat", "transport", "canvas", "route.go")
	if _, err := os.Stat(aiChatCanvasRoute); err != nil {
		t.Fatalf("expected ai chat canvas route transport to exist: %v", err)
	}
	routesCanvas := readRouteLineSource(t, root, "internal/server/routes_canvas.go")
	mustContainRouteLine(t, routesCanvas, `aichatcanvas "admin_back_go/internal/module/ai/chat/transport/canvas"`)
	mustContainRouteLine(t, routesCanvas, `aichatcanvas.RegisterRoutes(router, retired.AIChat)`)
}

func TestAIChatCanvasTransportDoesNotImportCanvasModule(t *testing.T) {
	root := backendRoot(t)
	transportRoot := filepath.Join(root, "internal", "module", "ai", "chat", "transport", "canvas")
	var offenders []string
	err := filepath.WalkDir(transportRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), `admin_back_go/internal/module/canvas`) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ai chat canvas transport: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("ai/chat canvas transport must not import canvas module; offenders=%v", offenders)
	}
}

func TestCanvasModuleProductionCodeDoesNotOwnAIChatRuntime(t *testing.T) {
	root := backendRoot(t)
	canvasRoot := filepath.Join(root, "internal", "module", "canvas")
	forbidden := []string{"ChatCompletion(", "ChatCompletionInput", "ChatCompletionResponse", "TextRuntimeService", "NewTextRuntimeService", "TextGormRepository", "NewTextGormRepository", "AgentForTextRuntime"}
	var offenders []string
	err := filepath.WalkDir(canvasRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(rel)+" contains "+token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk canvas module: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("canvas module production code must not own AI chat runtime:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestCanvasAIVideoRoutesOwnedByAIVideoTransport(t *testing.T) {
	root := backendRoot(t)
	canvasRoute := readRouteLineSource(t, root, "internal/module/canvas/transport/canvas/route.go")
	mustNotContainRouteLine(t, canvasRoute, `"/ai/videos`)

	aiVideoCanvasRoute := filepath.Join(root, "internal", "module", "ai", "video", "transport", "canvas", "route.go")
	if _, err := os.Stat(aiVideoCanvasRoute); err != nil {
		t.Fatalf("expected ai video canvas route transport to exist: %v", err)
	}
	routesCanvas := readRouteLineSource(t, root, "internal/server/routes_canvas.go")
	mustContainRouteLine(t, routesCanvas, `aivideocanvas "admin_back_go/internal/module/ai/video/transport/canvas"`)
	mustContainRouteLine(t, routesCanvas, `aivideocanvas.RegisterRoutes(router, retired.AIVideo)`)
}

func TestCanvasAIAudioRoutesOwnedByAIAudioTransport(t *testing.T) {
	root := backendRoot(t)
	canvasRoute := readRouteLineSource(t, root, "internal/module/canvas/transport/canvas/route.go")
	mustNotContainRouteLine(t, canvasRoute, `"/ai/audios`)

	aiAudioCanvasRoute := filepath.Join(root, "internal", "module", "ai", "audio", "transport", "canvas", "route.go")
	if _, err := os.Stat(aiAudioCanvasRoute); err != nil {
		t.Fatalf("expected ai audio canvas route transport to exist: %v", err)
	}
	routesCanvas := readRouteLineSource(t, root, "internal/server/routes_canvas.go")
	mustContainRouteLine(t, routesCanvas, `aiaudiocanvas "admin_back_go/internal/module/ai/audio/transport/canvas"`)
	mustContainRouteLine(t, routesCanvas, `aiaudiocanvas.RegisterRoutes(router, retired.AIAudio)`)
}

func TestAIAudioCanvasTransportDoesNotImportCanvasModule(t *testing.T) {
	root := backendRoot(t)
	transportRoot := filepath.Join(root, "internal", "module", "ai", "audio", "transport", "canvas")
	var offenders []string
	err := filepath.WalkDir(transportRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), `admin_back_go/internal/module/canvas`) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ai audio canvas transport: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("ai/audio canvas transport must not import canvas module; offenders=%v", offenders)
	}
}

func TestCanvasModuleProductionCodeDoesNotOwnAIAudioRuntime(t *testing.T) {
	root := backendRoot(t)
	canvasRoot := filepath.Join(root, "internal", "module", "canvas")
	forbidden := []string{"GenerateAudio(", "AudioGenerationInput", "AudioGenerationResponse", "AudioRuntimeService", "NewAudioRuntimeService", "AudioGormRepository", "NewAudioGormRepository", "AgentForAudioRuntime", "AudioEngineFactory", "AudioRepository"}
	var offenders []string
	err := filepath.WalkDir(canvasRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(rel)+" contains "+token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk canvas module: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("canvas module production code must not own AI audio runtime:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestAIVideoCanvasTransportDoesNotImportCanvasModule(t *testing.T) {
	root := backendRoot(t)
	transportRoot := filepath.Join(root, "internal", "module", "ai", "video", "transport", "canvas")
	var offenders []string
	err := filepath.WalkDir(transportRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(body), `admin_back_go/internal/module/canvas`) {
			rel, _ := filepath.Rel(root, path)
			offenders = append(offenders, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk ai video canvas transport: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("ai/video canvas transport must not import canvas module; offenders=%v", offenders)
	}
}

func TestCanvasModuleProductionCodeDoesNotOwnAIVideoRuntime(t *testing.T) {
	root := backendRoot(t)
	canvasRoot := filepath.Join(root, "internal", "module", "canvas")
	forbidden := []string{"GenerateVideo(", "VideoGenerationInput", "VideoGenerationResponse", "VideoStatusResponse", "VideoRuntimeService", "NewVideoRuntimeService", "VideoGormRepository", "NewVideoGormRepository", "AgentForVideoRuntime", "VideoEngineFactory", "VideoRepository", "VideoTask) TableName"}
	var offenders []string
	err := filepath.WalkDir(canvasRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				rel, _ := filepath.Rel(root, path)
				offenders = append(offenders, filepath.ToSlash(rel)+" contains "+token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk canvas module: %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("canvas module production code must not own AI video runtime:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func readRouteLineSource(t *testing.T, root string, rel string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(body)
}

func mustContainRouteLine(t *testing.T, text string, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("expected route-line source to contain %q", want)
	}
}

func mustNotContainRouteLine(t *testing.T, text string, forbidden string) {
	t.Helper()
	if strings.Contains(text, forbidden) {
		t.Fatalf("route-line source must not contain %q", forbidden)
	}
}
