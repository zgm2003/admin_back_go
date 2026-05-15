package ai

import "context"

type ImageAsset struct {
	Name     string
	MimeType string
	Data     []byte
}

type ImageInput struct {
	Model             string
	Prompt            string
	Size              string
	Quality           string
	OutputFormat      string
	OutputCompression *int
	Moderation        string
	N                 int
	InputAssets       []ImageAsset
	MaskAsset         *ImageAsset
}

type GeneratedImage struct {
	B64JSON       string
	URL           string
	MimeType      string
	RevisedPrompt string
}

type ImageResult struct {
	Images       []GeneratedImage
	ActualParams map[string]any
	RawResponse  []byte
}

type ImageEngine interface {
	GenerateImages(ctx context.Context, input ImageInput) (*ImageResult, error)
}
