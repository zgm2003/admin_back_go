package officialmodel

import (
	"errors"
	"reflect"
	"testing"
)

func TestOfficialCatalogRejectsUnprovedOrInconsistentCapabilities(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Model)
	}{
		{
			name: "image modality without image input contract",
			mutate: func(model *Model) {
				model.Capabilities.InputModalities = []string{ModalityText, ModalityImage}
			},
		},
		{
			name: "image input without image modality",
			mutate: func(model *Model) {
				model.Capabilities.ImageInput = &ImageInputCapability{MIMETypes: []string{"image/png"}, MaxFiles: 1, MaxBytes: 1024}
			},
		},
		{
			name: "native file boolean without modality",
			mutate: func(model *Model) {
				model.Capabilities.NativeFileInput = true
			},
		},
		{
			name: "native file modality without boolean",
			mutate: func(model *Model) {
				model.Capabilities.InputModalities = []string{ModalityText, ModalityFile}
			},
		},
		{
			name: "unsupported generation parameter",
			mutate: func(model *Model) {
				model.Capabilities.SupportedParameters = []string{"top_p"}
			},
		},
		{
			name: "invalid lifecycle",
			mutate: func(model *Model) {
				model.LifecycleStatus = LifecycleStatus("unknown")
			},
		},
		{
			name: "output exceeds context",
			mutate: func(model *Model) {
				model.MaxOutputTokens = model.ContextWindowTokens + 1
			},
		},
		{
			name: "missing review source",
			mutate: func(model *Model) {
				model.ReviewAfter = ""
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			model := validCatalogModel("model-a")
			test.mutate(&model)
			if _, err := NewCatalog("test-v1", []Model{model}); !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestEffectiveCapabilitiesCanOnlyNarrowOfficialFacts(t *testing.T) {
	official := Capabilities{
		InputModalities:          []string{ModalityText, ModalityImage},
		OutputModalities:         []string{ModalityText},
		SupportsStreaming:        true,
		SupportsTools:            true,
		SupportsStructuredOutput: true,
		SupportedParameters:      []string{ParameterTemperature},
		ImageInput: &ImageInputCapability{
			MIMETypes: []string{"image/jpeg", "image/png", "image/webp"},
			MaxFiles:  5,
			MaxBytes:  10 << 20,
		},
	}
	transport := official
	transport.InputModalities = []string{ModalityText, ModalityImage, ModalityAudio}
	transport.SupportedParameters = []string{ParameterTemperature, "top_p"}
	agentPolicy := official
	agentPolicy.SupportsTools = false
	agentPolicy.ImageInput = &ImageInputCapability{
		MIMETypes: []string{"image/jpeg", "image/png", "image/gif"},
		MaxFiles:  3,
		MaxBytes:  8 << 20,
	}
	platform := official
	platform.SupportsStructuredOutput = false
	platform.ImageInput = &ImageInputCapability{
		MIMETypes: []string{"image/jpeg", "image/png"},
		MaxFiles:  5,
		MaxBytes:  4 << 20,
	}

	got := EffectiveCapabilities(EffectiveCapabilityInput{
		Official:             official,
		Transport:            transport,
		ProviderRouteEnabled: true,
		AgentPolicy:          agentPolicy,
		PlatformImplemented:  platform,
	})
	if !reflect.DeepEqual(got.InputModalities, []string{ModalityText, ModalityImage}) ||
		!reflect.DeepEqual(got.OutputModalities, []string{ModalityText}) ||
		!reflect.DeepEqual(got.SupportedParameters, []string{ParameterTemperature}) {
		t.Fatalf("effective sets expanded or changed order: %#v", got)
	}
	if !got.SupportsStreaming || got.SupportsTools || got.SupportsStructuredOutput || got.NativeFileInput {
		t.Fatalf("effective flags were not narrowed: %#v", got)
	}
	if got.ImageInput == nil || !reflect.DeepEqual(got.ImageInput.MIMETypes, []string{"image/jpeg", "image/png"}) ||
		got.ImageInput.MaxFiles != 3 || got.ImageInput.MaxBytes != 4<<20 {
		t.Fatalf("effective image limits=%#v", got.ImageInput)
	}

	disabled := EffectiveCapabilities(EffectiveCapabilityInput{
		Official:             official,
		Transport:            transport,
		ProviderRouteEnabled: false,
		AgentPolicy:          agentPolicy,
		PlatformImplemented:  platform,
	})
	if !reflect.DeepEqual(disabled, Capabilities{}) {
		t.Fatalf("disabled provider route exposed capabilities: %#v", disabled)
	}
}
