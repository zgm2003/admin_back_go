package ai

import "context"

type ImageAsset struct {
	Name            string
	MimeType        string
	StorageProvider string
	StorageKey      string
	SHA256          string
	SizeBytes       int64
	Data            []byte
}

type ImageInput struct {
	IdempotencyKey    string
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
	Images            []GeneratedImage
	ActualParams      map[string]any
	RawResponse       []byte
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
	UsageStatus       string
	Usage             UsageSnapshot `json:"usage"`
	DispatchState     string        `json:"dispatch_state"`
	ProviderRequestID string        `json:"provider_request_id,omitempty"`
	ResponseSHA256    [32]byte      `json:"response_sha256,omitempty"`
}

type ImageEngine interface {
	GenerateImages(ctx context.Context, input ImageInput) (*ImageResult, error)
}

const SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1 = "logical_request_and_attachment_bytes_v1"

type PreparedImageRequest struct {
	Body           []byte
	IdempotencyKey string
	InputAssets    []ImageAsset
	MaskAsset      *ImageAsset
}

type PreparedImageEngine interface {
	ImageEngine
	CapabilityProvider
	PrepareImageRequest(ImageInput) ([]byte, error)
	GeneratePreparedImages(context.Context, PreparedImageRequest) (*ImageResult, error)
}
