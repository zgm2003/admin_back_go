package canvas

import (
	"context"
	"testing"
)

type fakeCanvasRepository struct {
	agentsByScene map[string][]CanvasAgentOption
	agentScenes   []string
	err           error
}

func (f *fakeCanvasRepository) ListAgentsByScene(ctx context.Context, scene string) ([]CanvasAgentOption, error) {
	f.agentScenes = append(f.agentScenes, scene)
	if f.agentsByScene == nil {
		return nil, f.err
	}
	return f.agentsByScene[scene], f.err
}

func TestServicePublicSettingsReturnsPublicPolicyAndCanvasAgentScenes(t *testing.T) {
	auth := &fakeSettingsAuthPolicy{allowRegister: true}
	repo := &fakeCanvasRepository{agentsByScene: map[string][]CanvasAgentOption{
		canvasTextAgentScene:  {{ID: 7, Name: "文本助手", ModelID: "gpt-4.1-mini", ModelDisplayName: "GPT 4.1 Mini", Scene: canvasTextAgentScene}},
		canvasImageAgentScene: {{ID: 8, Name: "绘图助手", ModelID: "gpt-image-2", ModelDisplayName: "GPT Image", Scene: canvasImageAgentScene}},
		canvasVideoAgentScene: {{ID: 9, Name: "视频助手", ModelID: "video-model", ModelDisplayName: "Video", Scene: canvasVideoAgentScene}},
	}}
	svc := NewServiceWithSettings(repo, SettingsDependencies{AuthPolicy: auth})

	result, appErr := svc.PublicSettings(context.Background(), SettingsInput{UserID: 7})

	if appErr != nil {
		t.Fatalf("PublicSettings error=%#v", appErr)
	}
	if !result.AllowRegister || auth.platform != "canvas" {
		t.Fatalf("auth policy mismatch result=%#v platform=%q", result, auth.platform)
	}
	if len(result.Scenes) != 3 || result.Scenes[0] != canvasTextAgentScene || result.Scenes[2] != canvasVideoAgentScene {
		t.Fatalf("unexpected scenes: %#v", result.Scenes)
	}
	if len(result.Agents.Text) != 1 || result.Agents.Text[0].Scene != canvasTextAgentScene {
		t.Fatalf("text agents must come from canvas text scene, got %#v", result.Agents.Text)
	}
	if len(result.Agents.Image) != 1 || result.Agents.Image[0].Scene != canvasImageAgentScene {
		t.Fatalf("image agents must come from canvas image scene, got %#v", result.Agents.Image)
	}
	if len(result.Agents.Video) != 1 || result.Agents.Video[0].Scene != canvasVideoAgentScene {
		t.Fatalf("video agents must come from canvas video scene, got %#v", result.Agents.Video)
	}
	if len(repo.agentScenes) != 3 || repo.agentScenes[0] != canvasTextAgentScene || repo.agentScenes[1] != canvasImageAgentScene || repo.agentScenes[2] != canvasVideoAgentScene {
		t.Fatalf("settings must query canvas agent scenes, got %#v", repo.agentScenes)
	}
}

type fakeSettingsAuthPolicy struct {
	allowRegister bool
	platform      string
}

func (f *fakeSettingsAuthPolicy) AllowRegister(ctx context.Context, platform string) (bool, error) {
	f.platform = platform
	return f.allowRegister, nil
}
