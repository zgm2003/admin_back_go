package capability

import "testing"

func TestGenerationScenesAreCanonicalAndDistinct(t *testing.T) {
	want := map[string]string{
		"text":  "text_generate",
		"image": "image_generate",
		"video": "video_generate",
		"audio": "audio_generate",
	}
	got := map[string]string{
		"text":  SceneTextGenerate,
		"image": SceneImageGenerate,
		"video": SceneVideoGenerate,
		"audio": SceneAudioGenerate,
	}

	seen := make(map[string]string, len(got))
	for modality, scene := range got {
		if scene != want[modality] {
			t.Fatalf("%s scene = %q, want %q", modality, scene, want[modality])
		}
		if previous, ok := seen[scene]; ok {
			t.Fatalf("%s and %s share scene %q", previous, modality, scene)
		}
		seen[scene] = modality
	}
}
