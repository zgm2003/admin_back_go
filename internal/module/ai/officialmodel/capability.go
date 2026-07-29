package officialmodel

const (
	ModalityText  = "text"
	ModalityImage = "image"
	ModalityAudio = "audio"
	ModalityFile  = "file"

	ParameterTemperature = "temperature"
)

type ImageInputCapability struct {
	MIMETypes []string `json:"mime_types"`
	MaxFiles  int      `json:"max_files"`
	MaxBytes  int64    `json:"max_bytes"`
}

type Capabilities struct {
	InputModalities          []string              `json:"input_modalities"`
	OutputModalities         []string              `json:"output_modalities"`
	SupportsStreaming        bool                  `json:"supports_streaming"`
	SupportsTools            bool                  `json:"supports_tools"`
	SupportsStructuredOutput bool                  `json:"supports_structured_output"`
	SupportedParameters      []string              `json:"supported_parameters"`
	NativeFileInput          bool                  `json:"native_file_input"`
	ImageInput               *ImageInputCapability `json:"image_input,omitempty"`
}

func cloneCapabilities(value Capabilities) Capabilities {
	value.InputModalities = append([]string(nil), value.InputModalities...)
	value.OutputModalities = append([]string(nil), value.OutputModalities...)
	value.SupportedParameters = append([]string(nil), value.SupportedParameters...)
	if value.ImageInput != nil {
		image := *value.ImageInput
		image.MIMETypes = append([]string(nil), value.ImageInput.MIMETypes...)
		value.ImageInput = &image
	}
	return value
}
