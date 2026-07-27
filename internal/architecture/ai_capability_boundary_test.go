package architecture

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	aichat "admin_back_go/internal/module/ai/chat"
	aiimage "admin_back_go/internal/module/ai/image"
)

func TestRetainedAICapabilitiesAreTransportNeutral(t *testing.T) {
	root := backendRoot(t)
	forbidden := []string{
		strings.Join([]string{"canvas", "_"}, ""),
		strings.Join([]string{"canvas", "."}, ""),
		strings.Join([]string{"Platform", "Canvas"}, ""),
		strings.Join([]string{"Platform", "App"}, ""),
		strings.Join([]string{"Canvas", "Completion"}, ""),
		strings.Join([]string{"canvas", "_video_tasks"}, ""),
		"github.com/gin-gonic/gin",
		"/transport/app",
		"/transport/canvas",
	}

	var offenders []string
	for _, module := range []string{"agent", "chat", "image", "text"} {
		base := filepath.Join(root, "internal", "module", "ai", module)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "transport" {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(root, path)
			for _, token := range forbidden {
				if strings.Contains(string(body), token) {
					offenders = append(offenders, filepath.ToSlash(relative)+" contains "+token)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan retained AI capability %s: %v", module, err)
		}
	}
	if len(offenders) != 0 {
		sort.Strings(offenders)
		t.Fatalf("retained AI capability leaks retired product or HTTP transport language:\n  %s", strings.Join(offenders, "\n  "))
	}
}

func TestRetainedAICapabilityInputsRequirePlatformProvenance(t *testing.T) {
	for name, input := range map[string]any{
		"text":         aichat.TextCompletionInput{},
		"image":        aiimage.CreateInput{},
		"image_worker": aiimage.GenerateInput{},
	} {
		field, ok := reflect.TypeOf(input).FieldByName("Platform")
		if !ok || field.Type.Kind() != reflect.String {
			t.Errorf("%s input must carry explicit string Platform provenance", name)
		}
	}

}

func TestAIAudioVideoGenerationModulesAreAbsent(t *testing.T) {
	root := backendRoot(t)
	for _, relative := range []string{
		"internal/module/ai/audio",
		"internal/module/ai/video",
		"internal/shared/i18n/locales/en-US/aiaudio.yaml",
		"internal/shared/i18n/locales/en-US/aivideo.yaml",
		"internal/shared/i18n/locales/zh-CN/aiaudio.yaml",
		"internal/shared/i18n/locales/zh-CN/aivideo.yaml",
	} {
		mustNotExist(t, root, relative)
	}
}
