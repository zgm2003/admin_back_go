package officialmodel

type EffectiveCapabilityInput struct {
	Official             Capabilities
	Transport            Capabilities
	ProviderRouteEnabled bool
	AgentPolicy          Capabilities
	PlatformImplemented  Capabilities
}

func EffectiveCapabilities(input EffectiveCapabilityInput) Capabilities {
	if !input.ProviderRouteEnabled {
		return Capabilities{}
	}
	layers := []Capabilities{input.Transport, input.AgentPolicy, input.PlatformImplemented}
	effective := cloneCapabilities(input.Official)
	for _, layer := range layers {
		effective.InputModalities = intersectOrdered(effective.InputModalities, layer.InputModalities)
		effective.OutputModalities = intersectOrdered(effective.OutputModalities, layer.OutputModalities)
		effective.SupportedParameters = intersectOrdered(effective.SupportedParameters, layer.SupportedParameters)
		effective.SupportsStreaming = effective.SupportsStreaming && layer.SupportsStreaming
		effective.SupportsTools = effective.SupportsTools && layer.SupportsTools
		effective.SupportsStructuredOutput = effective.SupportsStructuredOutput && layer.SupportsStructuredOutput
		effective.NativeFileInput = effective.NativeFileInput && layer.NativeFileInput
		effective.ImageInput = intersectImageInput(effective.ImageInput, layer.ImageInput)
	}
	if !containsExact(effective.InputModalities, ModalityImage) {
		effective.ImageInput = nil
	}
	if !containsExact(effective.InputModalities, ModalityFile) {
		effective.NativeFileInput = false
	}
	return effective
}

func intersectOrdered(current []string, allowed []string) []string {
	if len(current) == 0 || len(allowed) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(current))
	for _, value := range current {
		if _, ok := set[value]; ok {
			out = append(out, value)
		}
	}
	return out
}

func intersectImageInput(current *ImageInputCapability, allowed *ImageInputCapability) *ImageInputCapability {
	if current == nil || allowed == nil {
		return nil
	}
	mimeTypes := intersectOrdered(current.MIMETypes, allowed.MIMETypes)
	maxFiles := minPositive(current.MaxFiles, allowed.MaxFiles)
	maxBytes := minPositive64(current.MaxBytes, allowed.MaxBytes)
	if len(mimeTypes) == 0 || maxFiles <= 0 || maxBytes <= 0 {
		return nil
	}
	return &ImageInputCapability{MIMETypes: mimeTypes, MaxFiles: maxFiles, MaxBytes: maxBytes}
}

func minPositive(left, right int) int {
	if left <= 0 || right <= 0 {
		return 0
	}
	if left < right {
		return left
	}
	return right
}

func minPositive64(left, right int64) int64 {
	if left <= 0 || right <= 0 {
		return 0
	}
	if left < right {
		return left
	}
	return right
}

func containsExact(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
