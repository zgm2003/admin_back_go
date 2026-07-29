package capability

import (
	"errors"

	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/module/ai/officialmodel"
)

var ErrTransportCapabilitiesUnavailable = errors.New("AI transport capabilities are unavailable")

// EffectiveChatCapabilities applies the transport and currently implemented
// Admin chat surface to immutable official model capabilities.
func EffectiveChatCapabilities(
	official officialmodel.Capabilities,
	transport infraai.CapabilityMetadata,
	providerRouteEnabled bool,
) (officialmodel.Capabilities, error) {
	if len(transport.InputModalities) == 0 || len(transport.OutputModalities) == 0 {
		return officialmodel.Capabilities{}, ErrTransportCapabilitiesUnavailable
	}
	return officialmodel.EffectiveCapabilities(officialmodel.EffectiveCapabilityInput{
		Official:             official,
		Transport:            transportLayer(official, transport),
		ProviderRouteEnabled: providerRouteEnabled,
		AgentPolicy:          official,
		PlatformImplemented:  adminChatPlatformCapabilities(),
	}), nil
}

func transportLayer(official officialmodel.Capabilities, metadata infraai.CapabilityMetadata) officialmodel.Capabilities {
	layer := officialmodel.Capabilities{
		InputModalities:          append([]string(nil), metadata.InputModalities...),
		OutputModalities:         append([]string(nil), metadata.OutputModalities...),
		SupportsStreaming:        metadata.SupportsStreaming,
		SupportsTools:            metadata.SupportsTools,
		SupportsStructuredOutput: metadata.SupportsStructuredOutput,
		SupportedParameters:      append([]string(nil), metadata.SupportedParameters...),
		NativeFileInput:          contains(metadata.InputModalities, officialmodel.ModalityFile),
	}
	if contains(metadata.InputModalities, officialmodel.ModalityImage) && official.ImageInput != nil {
		image := *official.ImageInput
		image.MIMETypes = append([]string(nil), official.ImageInput.MIMETypes...)
		layer.ImageInput = &image
	}
	return layer
}

func adminChatPlatformCapabilities() officialmodel.Capabilities {
	return officialmodel.Capabilities{
		InputModalities:          []string{officialmodel.ModalityText, officialmodel.ModalityImage},
		OutputModalities:         []string{officialmodel.ModalityText},
		SupportsStreaming:        true,
		SupportsTools:            true,
		SupportsStructuredOutput: false,
		SupportedParameters:      []string{officialmodel.ParameterTemperature},
		ImageInput: &officialmodel.ImageInputCapability{
			MIMETypes: []string{"image/jpeg", "image/png", "image/webp", "image/gif"},
			MaxFiles:  5,
			MaxBytes:  10 << 20,
		},
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
