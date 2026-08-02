package contextengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	infraai "admin_back_go/internal/infra/ai"
)

var errInvalidCanonicalHash = errors.New("invalid canonical hash input")

type DenseDistance string

const (
	DenseDistanceCosine DenseDistance = "cosine"
	DenseDistanceDot    DenseDistance = "dot"
	DenseDistanceEuclid DenseDistance = "euclid"

	SparseEncoderUnicodeLexicalV1 = "unicode_lexical_v1"
	SparseEncoderVersionV1        = "1"
)

func (distance DenseDistance) Validate() error {
	switch distance {
	case DenseDistanceCosine, DenseDistanceDot, DenseDistanceEuclid:
		return nil
	}
	return invalidValue("dense distance", string(distance))
}

type ContextProfileHashInput struct {
	ID                       uint64
	Name                     string
	Status                   string
	IndexState               ProfileIndexState
	ActiveGeneration         *uint64
	TargetGeneration         *uint64
	VerifiedUnixMS           int64
	EmbeddingProviderModelID uint64
	EmbeddingDimensions      uint32
	EmbeddingMaxInputTokens  int64
	EmbeddingTokenCounterID  string
	DenseDistance            DenseDistance
	DenseMinScore            FixedScore
	SparseEncoder            string
	SparseEncoderVersion     string
	RerankerProviderModelID  *uint64
	RerankerMinScore         *FixedScore
	MemoryProviderModelID    *uint64
}

type contextProfileCanonicalV1 struct {
	Schema                   string  `json:"schema"`
	EmbeddingProviderModelID uint64  `json:"embedding_provider_model_id"`
	EmbeddingDimensions      uint32  `json:"embedding_dimensions"`
	EmbeddingMaxInputTokens  int64   `json:"embedding_max_input_tokens"`
	EmbeddingTokenCounterID  string  `json:"embedding_token_counter_id"`
	DenseDistance            string  `json:"dense_distance"`
	DenseMinScore            string  `json:"dense_min_score"`
	SparseEncoder            string  `json:"sparse_encoder"`
	SparseEncoderVersion     string  `json:"sparse_encoder_version"`
	RerankerProviderModelID  *uint64 `json:"reranker_provider_model_id,omitempty"`
	RerankerMinScore         *string `json:"reranker_min_score,omitempty"`
	MemoryProviderModelID    *uint64 `json:"memory_provider_model_id,omitempty"`
}

func HashContextProfile(input ContextProfileHashInput) ([sha256.Size]byte, error) {
	if input.EmbeddingProviderModelID == 0 || input.EmbeddingDimensions == 0 || input.EmbeddingMaxInputTokens <= 0 ||
		input.SparseEncoder != SparseEncoderUnicodeLexicalV1 || input.SparseEncoderVersion != SparseEncoderVersionV1 {
		return [sha256.Size]byte{}, fmt.Errorf("%w: context profile", errInvalidCanonicalHash)
	}
	if _, err := infraai.ResolveTokenCounter(input.EmbeddingTokenCounterID); err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: token counter: %v", errInvalidCanonicalHash, err)
	}
	if err := input.DenseDistance.Validate(); err != nil {
		return [sha256.Size]byte{}, err
	}
	if err := input.DenseMinScore.Validate(); err != nil {
		return [sha256.Size]byte{}, err
	}
	if (input.RerankerProviderModelID == nil) != (input.RerankerMinScore == nil) ||
		invalidOptionalPositiveID(input.RerankerProviderModelID) || invalidOptionalPositiveID(input.MemoryProviderModelID) {
		return [sha256.Size]byte{}, fmt.Errorf("%w: profile provider model", errInvalidCanonicalHash)
	}
	if input.RerankerMinScore != nil {
		if err := input.RerankerMinScore.Validate(); err != nil {
			return [sha256.Size]byte{}, err
		}
	}
	payload := contextProfileCanonicalV1{
		Schema:                   "context_profile_v1",
		EmbeddingProviderModelID: input.EmbeddingProviderModelID,
		EmbeddingDimensions:      input.EmbeddingDimensions,
		EmbeddingMaxInputTokens:  input.EmbeddingMaxInputTokens,
		EmbeddingTokenCounterID:  input.EmbeddingTokenCounterID,
		DenseDistance:            string(input.DenseDistance),
		DenseMinScore:            input.DenseMinScore.String(),
		SparseEncoder:            input.SparseEncoder,
		SparseEncoderVersion:     input.SparseEncoderVersion,
		RerankerProviderModelID:  clonePointer(input.RerankerProviderModelID),
		RerankerMinScore:         fixedScoreStringPointer(input.RerankerMinScore),
		MemoryProviderModelID:    clonePointer(input.MemoryProviderModelID),
	}
	return canonicalSHA256(payload)
}

type ModelCapabilityHashInput struct {
	ProviderID          uint64
	ProviderModelID     uint64
	RequestedModelID    string
	CanonicalModelID    string
	APIProtocol         string
	ContextWindowTokens int64
	MaxOutputTokens     int64
	TokenCounterID      string
	InputModalities     []string
	OutputModalities    []string
	SupportedParameters []string
	SupportsTools       bool
	NativeFileInput     bool
	ImageInput          bool
}

type modelCapabilityCanonicalV1 struct {
	Schema              string   `json:"schema"`
	ProviderID          uint64   `json:"provider_id"`
	ProviderModelID     uint64   `json:"provider_model_id"`
	RequestedModelID    string   `json:"requested_model_id"`
	CanonicalModelID    string   `json:"canonical_model_id"`
	APIProtocol         string   `json:"api_protocol"`
	ContextWindowTokens int64    `json:"context_window_tokens"`
	MaxOutputTokens     int64    `json:"max_output_tokens"`
	TokenCounterID      string   `json:"token_counter_id"`
	InputModalities     []string `json:"input_modalities"`
	OutputModalities    []string `json:"output_modalities,omitempty"`
	SupportedParameters []string `json:"supported_parameters,omitempty"`
	SupportsTools       bool     `json:"supports_tools"`
	NativeFileInput     bool     `json:"native_file_input"`
	ImageInput          bool     `json:"image_input"`
}

func HashModelCapability(input ModelCapabilityHashInput) ([sha256.Size]byte, error) {
	if input.ProviderID == 0 || input.ProviderModelID == 0 || !validHashString(input.RequestedModelID) ||
		!validHashString(input.CanonicalModelID) || input.ContextWindowTokens <= 0 || input.MaxOutputTokens <= 0 ||
		input.MaxOutputTokens > input.ContextWindowTokens {
		return [sha256.Size]byte{}, fmt.Errorf("%w: model capability", errInvalidCanonicalHash)
	}
	if input.APIProtocol != APIProtocolChatCompletions && input.APIProtocol != APIProtocolResponses {
		return [sha256.Size]byte{}, fmt.Errorf("%w: API protocol", errInvalidCanonicalHash)
	}
	if _, err := infraai.ResolveTokenCounter(input.TokenCounterID); err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: token counter: %v", errInvalidCanonicalHash, err)
	}
	inputModalities, err := canonicalStringSet(input.InputModalities, true)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	outputModalities, err := canonicalStringSet(input.OutputModalities, false)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	parameters, err := canonicalStringSet(input.SupportedParameters, false)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	if err := validateModelCapabilityEnums(inputModalities, outputModalities, parameters); err != nil {
		return [sha256.Size]byte{}, err
	}
	return canonicalSHA256(modelCapabilityCanonicalV1{
		Schema:              "model_capability_v1",
		ProviderID:          input.ProviderID,
		ProviderModelID:     input.ProviderModelID,
		RequestedModelID:    input.RequestedModelID,
		CanonicalModelID:    input.CanonicalModelID,
		APIProtocol:         input.APIProtocol,
		ContextWindowTokens: input.ContextWindowTokens,
		MaxOutputTokens:     input.MaxOutputTokens,
		TokenCounterID:      input.TokenCounterID,
		InputModalities:     inputModalities,
		OutputModalities:    outputModalities,
		SupportedParameters: parameters,
		SupportsTools:       input.SupportsTools,
		NativeFileInput:     input.NativeFileInput,
		ImageInput:          input.ImageInput,
	})
}

type InputFingerprintHashInput struct {
	PolicyVersion         string
	AgentID               uint64
	AgentSHA256           [sha256.Size]byte
	ProviderID            uint64
	ProviderSHA256        [sha256.Size]byte
	ProviderModelID       uint64
	ModelID               string
	ModelCapabilitySHA256 [sha256.Size]byte
	Profile               *ProfileSnapshot
	Messages              []FingerprintMessage
	Bindings              []FingerprintBinding
	Tools                 []FingerprintTool
	Generation            FingerprintGeneration
}

type FingerprintMessage struct {
	ID            uint64
	Role          infraai.MessageRole
	ContentSHA256 [sha256.Size]byte
	Attachments   []FingerprintAttachment
}

type FingerprintAttachment struct {
	Ordinal   uint32
	Kind      AttachmentKind
	URL       string
	ObjectKey string
	ETag      string
	Size      int64
	MIMEType  string
	Filename  string
}

type FingerprintBinding struct {
	ID      uint64
	SpaceID uint64
	SHA256  [sha256.Size]byte
}

type FingerprintTool struct {
	ID               uint64
	Name             string
	DefinitionSHA256 [sha256.Size]byte
}

type FingerprintGeneration struct {
	Temperature              *FixedScore
	EffectiveMaxOutputTokens int64
}

type inputFingerprintCanonicalV1 struct {
	Schema                string                         `json:"schema"`
	PolicyVersion         string                         `json:"policy_version"`
	AgentID               uint64                         `json:"agent_id"`
	AgentSHA256           string                         `json:"agent_sha256"`
	ProviderID            uint64                         `json:"provider_id"`
	ProviderSHA256        string                         `json:"provider_sha256"`
	ProviderModelID       uint64                         `json:"provider_model_id"`
	ModelID               string                         `json:"model_id"`
	ModelCapabilitySHA256 string                         `json:"model_capability_sha256"`
	Profile               *fingerprintProfileCanonical   `json:"profile,omitempty"`
	Messages              []fingerprintMessageCanonical  `json:"messages"`
	Bindings              []fingerprintBindingCanonical  `json:"bindings,omitempty"`
	Tools                 []fingerprintToolCanonical     `json:"tools,omitempty"`
	Generation            fingerprintGenerationCanonical `json:"generation"`
}

type fingerprintProfileCanonical struct {
	ID              uint64  `json:"id"`
	SHA256          string  `json:"sha256"`
	IndexGeneration *uint64 `json:"index_generation,omitempty"`
}

type fingerprintMessageCanonical struct {
	ID            uint64                           `json:"id"`
	Role          string                           `json:"role"`
	ContentSHA256 string                           `json:"content_sha256"`
	Attachments   []fingerprintAttachmentCanonical `json:"attachments,omitempty"`
}

type fingerprintAttachmentCanonical struct {
	Ordinal   uint32 `json:"ordinal"`
	Kind      string `json:"kind"`
	URL       string `json:"url,omitempty"`
	ObjectKey string `json:"object_key"`
	ETag      string `json:"etag"`
	Size      int64  `json:"size"`
	MIMEType  string `json:"mime_type"`
	Filename  string `json:"filename"`
}

type fingerprintBindingCanonical struct {
	ID      uint64 `json:"id"`
	SpaceID uint64 `json:"space_id"`
	SHA256  string `json:"sha256"`
}

type fingerprintToolCanonical struct {
	ID               uint64 `json:"id"`
	Name             string `json:"name"`
	DefinitionSHA256 string `json:"definition_sha256"`
}

type fingerprintGenerationCanonical struct {
	Temperature              *string `json:"temperature,omitempty"`
	EffectiveMaxOutputTokens int64   `json:"effective_max_output_tokens,omitempty"`
}

func HashInputFingerprint(input InputFingerprintHashInput) ([sha256.Size]byte, error) {
	if !validIdentifier(input.PolicyVersion, 64) || input.AgentID == 0 || input.ProviderID == 0 || input.ProviderModelID == 0 ||
		isZeroSHA256(input.AgentSHA256) || isZeroSHA256(input.ProviderSHA256) || isZeroSHA256(input.ModelCapabilitySHA256) ||
		!validHashString(input.ModelID) || len(input.Messages) == 0 || input.Generation.EffectiveMaxOutputTokens < 0 {
		return [sha256.Size]byte{}, fmt.Errorf("%w: input fingerprint", errInvalidCanonicalHash)
	}
	if input.Generation.Temperature != nil {
		if err := input.Generation.Temperature.Validate(); err != nil {
			return [sha256.Size]byte{}, err
		}
	}

	profile, err := canonicalFingerprintProfile(input.Profile)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	messages, err := canonicalFingerprintMessages(input.Messages)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	bindings, err := canonicalFingerprintBindings(input.Bindings)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	tools, err := canonicalFingerprintTools(input.Tools)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return canonicalSHA256(inputFingerprintCanonicalV1{
		Schema:                "context_input_fingerprint_v1",
		PolicyVersion:         input.PolicyVersion,
		AgentID:               input.AgentID,
		AgentSHA256:           hexSHA256(input.AgentSHA256),
		ProviderID:            input.ProviderID,
		ProviderSHA256:        hexSHA256(input.ProviderSHA256),
		ProviderModelID:       input.ProviderModelID,
		ModelID:               input.ModelID,
		ModelCapabilitySHA256: hexSHA256(input.ModelCapabilitySHA256),
		Profile:               profile,
		Messages:              messages,
		Bindings:              bindings,
		Tools:                 tools,
		Generation: fingerprintGenerationCanonical{
			Temperature:              fixedScoreStringPointer(input.Generation.Temperature),
			EffectiveMaxOutputTokens: input.Generation.EffectiveMaxOutputTokens,
		},
	})
}

func canonicalFingerprintProfile(profile *ProfileSnapshot) (*fingerprintProfileCanonical, error) {
	if profile == nil {
		return nil, nil
	}
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	return &fingerprintProfileCanonical{
		ID: profile.ID, SHA256: hexSHA256(profile.SHA256), IndexGeneration: clonePointer(profile.IndexGeneration),
	}, nil
}

func canonicalFingerprintMessages(messages []FingerprintMessage) ([]fingerprintMessageCanonical, error) {
	result := make([]fingerprintMessageCanonical, 0, len(messages))
	seen := make(map[uint64]struct{}, len(messages))
	for _, message := range messages {
		if message.ID == 0 || isZeroSHA256(message.ContentSHA256) {
			return nil, fmt.Errorf("%w: fingerprint message", errInvalidCanonicalHash)
		}
		if _, exists := seen[message.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate fingerprint message", errInvalidCanonicalHash)
		}
		seen[message.ID] = struct{}{}
		if err := message.Role.Validate(); err != nil {
			return nil, err
		}
		attachments := make([]fingerprintAttachmentCanonical, 0, len(message.Attachments))
		for index, attachment := range message.Attachments {
			if attachment.Ordinal != uint32(index+1) {
				return nil, fmt.Errorf("%w: attachment ordinal", errInvalidCanonicalHash)
			}
			facts := ContextAttachmentV1{
				Kind: attachment.Kind, URL: attachment.URL, ObjectKey: attachment.ObjectKey, ETag: attachment.ETag,
				Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
			}
			if err := facts.Validate(); err != nil {
				return nil, err
			}
			attachments = append(attachments, fingerprintAttachmentCanonical{
				Ordinal: attachment.Ordinal, Kind: string(attachment.Kind), URL: attachment.URL, ObjectKey: attachment.ObjectKey,
				ETag: attachment.ETag, Size: attachment.Size, MIMEType: attachment.MIMEType, Filename: attachment.Filename,
			})
		}
		result = append(result, fingerprintMessageCanonical{
			ID: message.ID, Role: string(message.Role), ContentSHA256: hexSHA256(message.ContentSHA256), Attachments: attachments,
		})
	}
	return result, nil
}

func canonicalFingerprintBindings(bindings []FingerprintBinding) ([]fingerprintBindingCanonical, error) {
	result := make([]fingerprintBindingCanonical, 0, len(bindings))
	seen := make(map[uint64]struct{}, len(bindings))
	for _, binding := range bindings {
		if binding.ID == 0 || binding.SpaceID == 0 || isZeroSHA256(binding.SHA256) {
			return nil, fmt.Errorf("%w: fingerprint binding", errInvalidCanonicalHash)
		}
		if _, exists := seen[binding.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate fingerprint binding", errInvalidCanonicalHash)
		}
		seen[binding.ID] = struct{}{}
		result = append(result, fingerprintBindingCanonical{ID: binding.ID, SpaceID: binding.SpaceID, SHA256: hexSHA256(binding.SHA256)})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].ID != result[right].ID {
			return result[left].ID < result[right].ID
		}
		if result[left].SpaceID != result[right].SpaceID {
			return result[left].SpaceID < result[right].SpaceID
		}
		return result[left].SHA256 < result[right].SHA256
	})
	return result, nil
}

func canonicalFingerprintTools(tools []FingerprintTool) ([]fingerprintToolCanonical, error) {
	result := make([]fingerprintToolCanonical, 0, len(tools))
	seenIDs := make(map[uint64]struct{}, len(tools))
	seenNames := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if tool.ID == 0 || !validHashString(tool.Name) || isZeroSHA256(tool.DefinitionSHA256) {
			return nil, fmt.Errorf("%w: fingerprint tool", errInvalidCanonicalHash)
		}
		if _, exists := seenIDs[tool.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate fingerprint tool ID", errInvalidCanonicalHash)
		}
		if _, exists := seenNames[tool.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate fingerprint tool name", errInvalidCanonicalHash)
		}
		seenIDs[tool.ID] = struct{}{}
		seenNames[tool.Name] = struct{}{}
		result = append(result, fingerprintToolCanonical{ID: tool.ID, Name: tool.Name, DefinitionSHA256: hexSHA256(tool.DefinitionSHA256)})
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].ID != result[right].ID {
			return result[left].ID < result[right].ID
		}
		if result[left].Name != result[right].Name {
			return result[left].Name < result[right].Name
		}
		return result[left].DefinitionSHA256 < result[right].DefinitionSHA256
	})
	return result, nil
}

type contextPlanCanonicalV1 struct {
	Schema                 string                       `json:"schema"`
	PolicyVersion          string                       `json:"policy_version"`
	InputFingerprintSHA256 string                       `json:"input_fingerprint_sha256"`
	ModelCapabilitySHA256  string                       `json:"model_capability_sha256"`
	Profile                *fingerprintProfileCanonical `json:"profile,omitempty"`
	APIProtocol            string                       `json:"api_protocol"`
	TokenCounterID         string                       `json:"token_counter_id"`
	Budget                 Budget                       `json:"budget"`
	RetrievalOutcome       RetrievalOutcome             `json:"retrieval_outcome"`
	Items                  []contextPlanItemCanonical   `json:"items"`
}

type contextPlanItemCanonical struct {
	Ordinal         uint32                `json:"ordinal"`
	Block           contextBlockCanonical `json:"block"`
	Decision        Decision              `json:"decision"`
	ExclusionReason *string               `json:"exclusion_reason,omitempty"`
	FusionScore     *string               `json:"fusion_score,omitempty"`
	RerankScore     *string               `json:"rerank_score,omitempty"`
	CitationKey     *string               `json:"citation_key,omitempty"`
}

type contextBlockCanonical struct {
	Kind            BlockKind              `json:"kind"`
	SourceType      string                 `json:"source_type"`
	SourceRef       string                 `json:"source_ref"`
	SourceSHA256    string                 `json:"source_sha256"`
	AtomicGroupKey  string                 `json:"atomic_group_key"`
	Required        bool                   `json:"required"`
	Priority        int32                  `json:"priority"`
	TokenUpperBound int64                  `json:"token_upper_bound"`
	ContentSnapshot *string                `json:"content_snapshot,omitempty"`
	Metadata        ContextBlockMetadataV1 `json:"metadata"`
}

func HashPlan(plan ContextPlan) ([sha256.Size]byte, error) {
	if plan.State != PlanReady {
		return [sha256.Size]byte{}, fmt.Errorf("%w: only ready plans are hashable", errInvalidCanonicalHash)
	}
	validated := plan
	if validated.PlanSHA256 == nil {
		placeholder := sha256.Sum256([]byte("context_plan_hash_pending_v1"))
		validated.PlanSHA256 = &placeholder
	}
	if err := validated.Validate(); err != nil {
		return [sha256.Size]byte{}, err
	}
	if _, err := infraai.ResolveTokenCounter(plan.TokenCounterID); err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: token counter: %v", errInvalidCanonicalHash, err)
	}
	profile, err := canonicalFingerprintProfile(plan.Profile)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	items := make([]contextPlanItemCanonical, 0, len(plan.Items))
	for _, item := range plan.Items {
		items = append(items, contextPlanItemCanonical{
			Ordinal: item.Ordinal,
			Block: contextBlockCanonical{
				Kind:            item.Block.Kind,
				SourceType:      item.Block.SourceType,
				SourceRef:       item.Block.SourceRef,
				SourceSHA256:    hexSHA256(item.Block.SourceSHA256),
				AtomicGroupKey:  item.Block.AtomicGroupKey,
				Required:        item.Block.Required,
				Priority:        item.Block.Priority,
				TokenUpperBound: item.Block.TokenUpperBound,
				ContentSnapshot: clonePointer(item.Block.ContentSnapshot),
				Metadata:        cloneContextBlockMetadata(item.Block.Metadata),
			},
			Decision:        item.Decision,
			ExclusionReason: exclusionReasonStringPointer(item.ExclusionReason),
			FusionScore:     fixedScoreStringPointer(item.FusionScore),
			RerankScore:     fixedScoreStringPointer(item.RerankScore),
			CitationKey:     clonePointer(item.CitationKey),
		})
	}
	return canonicalSHA256(contextPlanCanonicalV1{
		Schema:                 "context_plan_v1",
		PolicyVersion:          plan.PolicyVersion,
		InputFingerprintSHA256: hexSHA256(plan.InputFingerprintSHA256),
		ModelCapabilitySHA256:  hexSHA256(plan.ModelCapabilitySHA256),
		Profile:                profile,
		APIProtocol:            plan.APIProtocol,
		TokenCounterID:         plan.TokenCounterID,
		Budget:                 plan.Budget,
		RetrievalOutcome:       plan.RetrievalOutcome,
		Items:                  items,
	})
}

func canonicalSHA256(value any) ([sha256.Size]byte, error) {
	if value == nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: nil value", errInvalidCanonicalHash)
	}
	if err := validateCanonicalValue(reflect.ValueOf(value)); err != nil {
		return [sha256.Size]byte{}, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("%w: %v", errInvalidCanonicalHash, err)
	}
	return sha256.Sum256(encoded), nil
}

func validateCanonicalValue(value reflect.Value) error {
	if !value.IsValid() {
		return fmt.Errorf("%w: invalid value", errInvalidCanonicalHash)
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if value.IsNil() {
			return nil
		}
		return validateCanonicalValue(value.Elem())
	case reflect.Map:
		return fmt.Errorf("%w: maps are forbidden", errInvalidCanonicalHash)
	case reflect.Float32, reflect.Float64:
		return fmt.Errorf("%w: floats are forbidden", errInvalidCanonicalHash)
	case reflect.String:
		if !utf8.ValidString(value.String()) {
			return fmt.Errorf("%w: invalid UTF-8", errInvalidCanonicalHash)
		}
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			if err := validateCanonicalValue(value.Field(index)); err != nil {
				return err
			}
		}
	case reflect.Array, reflect.Slice:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return nil
		}
		for index := 0; index < value.Len(); index++ {
			if err := validateCanonicalValue(value.Index(index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func canonicalStringSet(values []string, required bool) ([]string, error) {
	if required && len(values) == 0 {
		return nil, fmt.Errorf("%w: empty string set", errInvalidCanonicalHash)
	}
	result := append([]string(nil), values...)
	for _, value := range result {
		if !validHashString(value) {
			return nil, fmt.Errorf("%w: string set value", errInvalidCanonicalHash)
		}
	}
	sort.Strings(result)
	for index := 1; index < len(result); index++ {
		if result[index] == result[index-1] {
			return nil, fmt.Errorf("%w: duplicate string set value", errInvalidCanonicalHash)
		}
	}
	return result, nil
}

func validateModelCapabilityEnums(inputModalities, outputModalities, parameters []string) error {
	for _, modalities := range [][]string{inputModalities, outputModalities} {
		for _, modality := range modalities {
			switch modality {
			case "text", "image", "audio", "file":
			default:
				return fmt.Errorf("%w: modality %q", errInvalidCanonicalHash, modality)
			}
		}
	}
	for _, parameter := range parameters {
		if parameter != "temperature" {
			return fmt.Errorf("%w: supported parameter %q", errInvalidCanonicalHash, parameter)
		}
	}
	return nil
}

func validHashString(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && utf8.ValidString(value)
}

func invalidOptionalPositiveID(value *uint64) bool {
	return value != nil && *value == 0
}

func fixedScoreStringPointer(score *FixedScore) *string {
	if score == nil {
		return nil
	}
	value := score.String()
	return &value
}

func exclusionReasonStringPointer(reason *ExclusionReason) *string {
	if reason == nil {
		return nil
	}
	value := string(*reason)
	return &value
}

func hexSHA256(value [sha256.Size]byte) string {
	return hex.EncodeToString(value[:])
}
