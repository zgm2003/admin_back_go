package capability

import (
	"reflect"
	"testing"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/officialmodel"
)

func TestEffectiveChatCapabilitiesExposeOnlyImplementedIntersection(t *testing.T) {
	official := officialmodel.Capabilities{
		InputModalities:          []string{officialmodel.ModalityText, officialmodel.ModalityImage, officialmodel.ModalityAudio, officialmodel.ModalityFile},
		OutputModalities:         []string{officialmodel.ModalityText, officialmodel.ModalityAudio},
		SupportsStreaming:        true,
		SupportsTools:            true,
		SupportsStructuredOutput: true,
		SupportedParameters:      []string{officialmodel.ParameterTemperature, "top_p"},
		NativeFileInput:          true,
		ImageInput: &officialmodel.ImageInputCapability{
			MIMETypes: []string{"image/jpeg", "image/png", "image/tiff"},
			MaxFiles:  8,
			MaxBytes:  20 << 20,
		},
	}
	transport := infraai.CapabilityMetadata{
		InputModalities:          []string{"text", "image", "file"},
		OutputModalities:         []string{"text"},
		SupportedParameters:      []string{"temperature", "top_p"},
		SupportsTools:            true,
		SupportsStreaming:        true,
		SupportsStructuredOutput: true,
	}

	got, err := EffectiveChatCapabilities(official, transport, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.InputModalities, []string{"text", "image", "file"}) ||
		!reflect.DeepEqual(got.OutputModalities, []string{"text"}) ||
		!reflect.DeepEqual(got.SupportedParameters, []string{"temperature"}) {
		t.Fatalf("unexpected effective modalities or parameters: %#v", got)
	}
	if !got.SupportsTools || !got.SupportsStreaming || got.SupportsStructuredOutput || !got.NativeFileInput {
		t.Fatalf("effective booleans widened platform support: %#v", got)
	}
	if got.ImageInput == nil || !reflect.DeepEqual(got.ImageInput.MIMETypes, []string{"image/jpeg", "image/png"}) ||
		got.ImageInput.MaxFiles != 5 || got.ImageInput.MaxBytes != 10<<20 {
		t.Fatalf("unexpected effective image limits: %#v", got.ImageInput)
	}
}

func TestEffectiveChatCapabilitiesFailClosedWithoutTransportOrRoute(t *testing.T) {
	official := officialmodel.Capabilities{InputModalities: []string{"text"}, OutputModalities: []string{"text"}}
	if _, err := EffectiveChatCapabilities(official, infraai.CapabilityMetadata{}, true); err == nil {
		t.Fatal("missing transport capabilities must fail closed")
	}
	got, err := EffectiveChatCapabilities(official, infraai.CapabilityMetadata{InputModalities: []string{"text"}, OutputModalities: []string{"text"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.InputModalities) != 0 || len(got.OutputModalities) != 0 {
		t.Fatalf("disabled route must expose no capabilities: %#v", got)
	}
}
