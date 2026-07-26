package runtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"reflect"
	"strings"
	"testing"
	"time"

	infraai "admin_back_go/internal/infra/ai"
	storagecos "admin_back_go/internal/infra/storage/cos"
	"admin_back_go/internal/module/ai/aigateway"
	"admin_back_go/internal/module/ai/billing"
	aiimage "admin_back_go/internal/module/ai/image"
	"admin_back_go/internal/module/ai/replycommand"
	airun "admin_back_go/internal/module/ai/run"
	walletmodule "admin_back_go/internal/module/payment/wallet"
)

type fakeImageGateway struct {
	calls          []string
	assembled      aigateway.PreparedCall
	reserved       aigateway.ProviderAttempt
	reserveInput   aigateway.ReserveAndPrepareInput
	dispatchInput  aigateway.ProviderAttempt
	assembleErr    error
	reserveErr     error
	dispatchErr    error
	dispatchResult aigateway.DispatchResult
}

func (gateway *fakeImageGateway) AssembleAndQuote(_ context.Context, _ aigateway.RunRequest) (aigateway.PreparedCall, error) {
	gateway.calls = append(gateway.calls, "assemble")
	return gateway.assembled, gateway.assembleErr
}

func (gateway *fakeImageGateway) ReserveAndPrepare(_ context.Context, input aigateway.ReserveAndPrepareInput) (aigateway.ProviderAttempt, error) {
	gateway.calls = append(gateway.calls, "reserve")
	gateway.reserveInput = input
	return gateway.reserved, gateway.reserveErr
}

func (gateway *fakeImageGateway) Dispatch(_ context.Context, attempt aigateway.ProviderAttempt) (aigateway.DispatchResult, error) {
	gateway.calls = append(gateway.calls, "dispatch")
	gateway.dispatchInput = attempt
	return gateway.dispatchResult, gateway.dispatchErr
}

func TestRunImageGatewayAttemptReserveFailureCreatesNoProviderCall(t *testing.T) {
	gateway := &fakeImageGateway{
		assembled:  aigateway.PreparedCall{RequestBody: []byte(`{"model":"gpt-image-2"}`)},
		reserveErr: &aigateway.Error{Code: aigateway.ErrCodeInsufficientBalance, Status: 409, Message: "low balance"},
	}

	_, err := runImageGatewayAttempt(context.Background(), gateway, aigateway.RunRequest{RunID: 51, UserID: 7, RequestID: "image-request-1"}, 1, false)

	var gatewayErr *aigateway.Error
	if !errors.As(err, &gatewayErr) || gatewayErr.Code != aigateway.ErrCodeInsufficientBalance {
		t.Fatalf("error = %v", err)
	}
	if !reflect.DeepEqual(gateway.calls, []string{"assemble", "reserve"}) {
		t.Fatalf("calls = %#v", gateway.calls)
	}
}

func TestRunImageGatewayAttemptPreparedRecoveryReusesLogicalBytesAndAttemptKey(t *testing.T) {
	logical := []byte("{\n  \"version\": \"openai_image_logical_v1\",\n  \"prompt\": \"exact bytes\"\n}")
	attempt := aigateway.ProviderAttempt{
		RunID: 51, AttemptNo: 1, IdempotencyKey: "run:51:attempt:1",
		PreparedRequest: append([]byte(nil), logical...),
	}
	gateway := &fakeImageGateway{reserved: attempt}

	_, err := runImageGatewayAttempt(context.Background(), gateway, aigateway.RunRequest{RunID: 51, UserID: 7, RequestID: "image-request-1"}, 1, true)

	if err != nil {
		t.Fatalf("runImageGatewayAttempt: %v", err)
	}
	if !reflect.DeepEqual(gateway.calls, []string{"reserve", "dispatch"}) {
		t.Fatalf("recovery calls = %#v", gateway.calls)
	}
	if gateway.reserveInput.NewCall != nil {
		t.Fatalf("prepared recovery rebuilt call: %#v", gateway.reserveInput.NewCall)
	}
	if !bytes.Equal(gateway.dispatchInput.PreparedRequest, logical) || gateway.dispatchInput.IdempotencyKey != attempt.IdempotencyKey {
		t.Fatalf("dispatch input = %#v", gateway.dispatchInput)
	}
}

type fakePreparedImageEngine struct {
	preparedBody  []byte
	preparedInput infraai.ImageInput
	dispatchInput infraai.PreparedImageRequest
	result        *infraai.ImageResult
	err           error
	capabilities  *infraai.CapabilityMetadata
}

func (engine *fakePreparedImageEngine) Capabilities() infraai.CapabilityMetadata {
	if engine.capabilities != nil {
		return *engine.capabilities
	}
	return infraai.CapabilityMetadata{
		SupportedUsageIdentities: []infraai.UsageIdentity{
			{Category: infraai.UsageCategoryInput, Unit: "token"},
			{Category: infraai.UsageCategoryOutput, Unit: "token"},
		},
		SafeInputUpperBoundStrategy: infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1,
		SupportsIdempotencyHeader:   true,
	}
}

func (engine *fakePreparedImageEngine) GenerateImages(context.Context, infraai.ImageInput) (*infraai.ImageResult, error) {
	panic("paid image dispatch must use GeneratePreparedImages")
}

func (engine *fakePreparedImageEngine) PrepareImageRequest(input infraai.ImageInput) ([]byte, error) {
	engine.preparedInput = input
	return append([]byte(nil), engine.preparedBody...), engine.err
}

func (engine *fakePreparedImageEngine) GeneratePreparedImages(_ context.Context, input infraai.PreparedImageRequest) (*infraai.ImageResult, error) {
	engine.dispatchInput = input
	return engine.result, engine.err
}

type fakeImageCandidateWriter struct {
	taskID    uint64
	attemptNo uint32
	result    *infraai.ImageResult
	ctxErr    error
	candidate string
	err       error
}

func (writer *fakeImageCandidateWriter) WriteImageCandidate(ctx context.Context, taskID uint64, attemptNo uint32, result *infraai.ImageResult) (string, error) {
	writer.taskID = taskID
	writer.attemptNo = attemptNo
	writer.result = result
	writer.ctxErr = ctx.Err()
	return writer.candidate, writer.err
}

func TestPaidImageAssemblerQuotesGenerationLogicalBytes(t *testing.T) {
	logical := []byte(`{"version":"openai_image_logical_v1","prompt":"draw"}`)
	engine := &fakePreparedImageEngine{preparedBody: logical}
	assembler := paidImageAssembler{transport: engine, input: infraai.ImageInput{Model: "gpt-image-2", Prompt: "draw", N: 1}}
	run := aigateway.RunSnapshot{RunID: 51, UserID: 7, ModelID: "gpt-image-2", PricingSnapshotJSON: testImagePricingSnapshotJSON()}

	call, err := assembler.AssembleAndQuote(context.Background(), run, aigateway.RunRequest{})

	if err != nil {
		t.Fatalf("AssembleAndQuote: %v", err)
	}
	wantInputBound := int64(len(logical)) + imageUpperBoundFramingBytes
	wantItems := []billing.UsageItem{
		{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: wantInputBound},
		{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 32768},
	}
	if !bytes.Equal(call.RequestBody, logical) || !reflect.DeepEqual(call.Quote.UpperBoundItems, wantItems) {
		t.Fatalf("prepared call = %#v", call)
	}
	if len(engine.preparedInput.InputAssets) != 0 || engine.preparedInput.MaskAsset != nil {
		t.Fatalf("generation unexpectedly carried edit assets: %#v", engine.preparedInput)
	}
}

func TestPaidImageAssemblerRejectsReferenceEditWithoutCategorizedUsage(t *testing.T) {
	asset := infraai.ImageAsset{
		Name: "input.png", MimeType: "image/png", StorageProvider: "cos", StorageKey: "immutable/input.png",
		SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SizeBytes: 7, Data: []byte("1234567"),
	}
	engine := &fakePreparedImageEngine{preparedBody: []byte(`{"prompt":"edit"}`)}
	assembler := paidImageAssembler{transport: engine, input: infraai.ImageInput{Model: "gpt-image-2", Prompt: "edit", N: 1, InputAssets: []infraai.ImageAsset{asset}}}

	_, err := assembler.AssembleAndQuote(context.Background(), aigateway.RunSnapshot{RunID: 51, UserID: 7, ModelID: "gpt-image-2", PricingSnapshotJSON: testImagePricingSnapshotJSON()}, aigateway.RunRequest{})
	if !errors.Is(err, ErrImageReferenceUsageUnavailable) {
		t.Fatalf("error=%v, want reference edit fail-closed", err)
	}
	if engine.preparedInput.Model != "" {
		t.Fatalf("reference edit reached prepared provider request: %#v", engine.preparedInput)
	}
}

func TestPaidImageAssemblerRejectsUnsafeCapabilityBeforePreparingAttempt(t *testing.T) {
	requiredUsage := []infraai.UsageIdentity{
		{Category: infraai.UsageCategoryInput, Unit: "token"},
		{Category: infraai.UsageCategoryOutput, Unit: "token"},
	}
	tests := []struct {
		name         string
		capabilities infraai.CapabilityMetadata
	}{
		{
			name: "missing idempotency",
			capabilities: infraai.CapabilityMetadata{
				SupportedUsageIdentities:    requiredUsage,
				SafeInputUpperBoundStrategy: infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1,
			},
		},
		{
			name: "unsupported upper bound",
			capabilities: infraai.CapabilityMetadata{
				SupportedUsageIdentities:    requiredUsage,
				SupportsIdempotencyHeader:   true,
				SafeInputUpperBoundStrategy: "unsupported",
			},
		},
		{
			name: "missing input usage",
			capabilities: infraai.CapabilityMetadata{
				SupportedUsageIdentities:    []infraai.UsageIdentity{{Category: infraai.UsageCategoryOutput, Unit: "token"}},
				SupportsIdempotencyHeader:   true,
				SafeInputUpperBoundStrategy: infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1,
			},
		},
		{
			name: "missing output usage",
			capabilities: infraai.CapabilityMetadata{
				SupportedUsageIdentities:    []infraai.UsageIdentity{{Category: infraai.UsageCategoryInput, Unit: "token"}},
				SupportsIdempotencyHeader:   true,
				SafeInputUpperBoundStrategy: infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := &fakePreparedImageEngine{preparedBody: []byte(`{"model":"gpt-image-2"}`), capabilities: &test.capabilities}
			assembler := paidImageAssembler{transport: engine, input: infraai.ImageInput{Model: "gpt-image-2", Prompt: "draw", N: 1}}
			run := aigateway.RunSnapshot{RunID: 51, UserID: 7, ModelID: "gpt-image-2", PricingSnapshotJSON: testImagePricingSnapshotJSON()}

			if _, err := assembler.AssembleAndQuote(context.Background(), run, aigateway.RunRequest{}); err == nil {
				t.Fatal("unsafe image capability reached quote/attempt preparation")
			}
			if engine.preparedInput.Model != "" {
				t.Fatalf("unsafe image capability prepared provider bytes: %#v", engine.preparedInput)
			}
		})
	}
}

func TestPreparedImageProviderProvesBoundAndReusesAttemptEvidence(t *testing.T) {
	logical := []byte(`{"version":"openai_image_logical_v1","prompt":"draw"}`)
	asset := infraai.ImageAsset{SizeBytes: 7, Data: []byte("1234567")}
	response := []byte(`{"data":[{"b64_json":"omitted-before-candidate"}]}`)
	responseHash := sha256.Sum256(response)
	engine := &fakePreparedImageEngine{result: &infraai.ImageResult{
		Images:            []infraai.GeneratedImage{{B64JSON: "provider-bytes", MimeType: "image/png"}},
		UsageStatus:       infraai.UsageStatusUnavailable,
		Usage:             infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable},
		DispatchState:     infraai.DispatchStateDispatched,
		ProviderRequestID: "image-provider-request-1",
		ResponseSHA256:    responseHash,
		RawResponse:       response,
	}}
	writer := &fakeImageCandidateWriter{candidate: `{"version":"ai_image_result_v1","outputs":[{"storage_provider":"cos","storage_key":"immutable/output.png"}]}`}
	provider := newPreparedImageProvider(engine, []infraai.ImageAsset{asset}, nil, writer, 81)
	inputBound := int64(len(logical)) + asset.SizeBytes + imageUpperBoundFramingBytes
	attempt := aigateway.ProviderAttempt{
		RunID: 51, AttemptNo: 1, IdempotencyKey: "run:51:attempt:1", PreparedRequest: logical, RequestSHA256: sha256.Sum256(logical),
		Quote: aigateway.QuoteEvidence{EffectiveMaxOutputTokens: 32768, UpperBoundItems: []billing.UsageItem{
			{Category: billing.UsageCategoryInputText, Unit: "token", Quantity: inputBound},
			{Category: billing.UsageCategoryOutputText, Unit: "token", Quantity: 32768},
		}},
	}

	proof, err := provider.ProvePreparedUpperBound(context.Background(), attempt)
	if err != nil {
		t.Fatalf("ProvePreparedUpperBound: %v", err)
	}
	if proof.RequestSHA256 != attempt.RequestSHA256 || proof.Strategy != infraai.SafeImageUpperBoundStrategyLogicalAndAttachmentBytesV1 || !reflect.DeepEqual(proof.Items, attempt.Quote.UpperBoundItems) {
		t.Fatalf("proof = %#v", proof)
	}
	result, err := provider.Dispatch(context.Background(), attempt)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !bytes.Equal(engine.dispatchInput.Body, logical) || engine.dispatchInput.IdempotencyKey != attempt.IdempotencyKey || !bytes.Equal(engine.dispatchInput.InputAssets[0].Data, asset.Data) {
		t.Fatalf("provider input = %#v", engine.dispatchInput)
	}
	if result.ResultCandidateJSON == nil || *result.ResultCandidateJSON != writer.candidate || writer.taskID != 81 || writer.attemptNo != 1 {
		t.Fatalf("dispatch result=%#v writer=%#v", result, writer)
	}
}

func TestPreparedImageProviderKeepsLeaseCancellationForCandidateWrite(t *testing.T) {
	engine := &fakePreparedImageEngine{result: &infraai.ImageResult{
		Images: []infraai.GeneratedImage{{B64JSON: "provider-bytes", MimeType: "image/png"}},
	}}
	writer := &fakeImageCandidateWriter{candidate: `{"version":"ai_image_result_v1","outputs":[{"storage_provider":"cos","storage_key":"immutable/output.png"}]}`}
	provider := newPreparedImageProvider(engine, nil, nil, writer, 81)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _ = provider.Dispatch(ctx, aigateway.ProviderAttempt{AttemptNo: 1})

	if !errors.Is(writer.ctxErr, context.Canceled) {
		t.Fatalf("candidate writer context error = %v, want context.Canceled", writer.ctxErr)
	}
}

type recordingImageObjectWriter struct {
	puts []storagecos.PutInput
}

func (writer *recordingImageObjectWriter) Put(_ context.Context, input storagecos.PutInput) error {
	input.Body = append([]byte(nil), input.Body...)
	writer.puts = append(writer.puts, input)
	return nil
}

func TestCOSImageCandidateWriterStoresImmutableReferencesWithoutBase64(t *testing.T) {
	body := testRuntimePNG(t, 2, 3)
	objects := &recordingImageObjectWriter{}
	writer := &cosImageCandidateWriter{
		objects: objects,
		destination: imageObjectDestination{
			SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket-1250000000", Region: "ap-guangzhou",
			BucketDomain: "https://cdn.test",
		},
	}
	result := &infraai.ImageResult{
		Images:       []infraai.GeneratedImage{{B64JSON: base64.StdEncoding.EncodeToString(body), MimeType: "image/png", RevisedPrompt: "revised"}},
		ActualParams: map[string]any{"size": "1024x1024", "nested": map[string]any{"b64_json": "PARAM_SECRET"}},
		RawResponse:  []byte(`{"data":[{"b64_json":"SECRET_IMAGE_BYTES"}]}`),
	}

	first, err := writer.WriteImageCandidate(context.Background(), 81, 1, result)
	if err != nil {
		t.Fatalf("WriteImageCandidate: %v", err)
	}
	second, err := writer.WriteImageCandidate(context.Background(), 81, 1, result)
	if err != nil {
		t.Fatalf("replay WriteImageCandidate: %v", err)
	}
	if first != second {
		t.Fatalf("candidate replay changed:\nfirst=%s\nsecond=%s", first, second)
	}
	if strings.Contains(first, "SECRET_IMAGE_BYTES") || strings.Contains(first, "PARAM_SECRET") || strings.Contains(first, base64.StdEncoding.EncodeToString(body)) || !strings.Contains(first, "[omitted]") {
		t.Fatalf("candidate leaked provider image bytes: %s", first)
	}
	candidate, err := decodeImageResultCandidate(first)
	if err != nil {
		t.Fatalf("decode candidate: %v", err)
	}
	if candidate.TaskID != 81 || candidate.AttemptNo != 1 || len(candidate.Outputs) != 1 {
		t.Fatalf("candidate = %#v", candidate)
	}
	output := candidate.Outputs[0]
	if output.StorageProvider != "cos" || output.StorageKey == "" || output.StorageURL == "" || output.Width != 2 || output.Height != 3 || output.SizeBytes != int64(len(body)) {
		t.Fatalf("output = %#v", output)
	}
	if len(objects.puts) != 2 || objects.puts[0].Key != objects.puts[1].Key || !bytes.Equal(objects.puts[0].Body, body) {
		t.Fatalf("object puts = %#v", objects.puts)
	}
}

type imageFinalizationCaptureStore struct {
	facts    aigateway.FinalizationFacts
	decision aigateway.SettlementDecision
}

func (store *imageFinalizationCaptureStore) WithLockedSettlement(_ context.Context, _ int64, decide func(aigateway.FinalizationFacts) (aigateway.SettlementDecision, error)) (aigateway.FinalizationApplyResult, error) {
	decision, err := decide(store.facts)
	if err != nil {
		return aigateway.FinalizationApplyResult{}, err
	}
	store.decision = decision
	return aigateway.FinalizationApplyResult{Applied: true}, nil
}

func TestImageMissingTerminalUsageReleasesHoldAndDiscardsOutputs(t *testing.T) {
	fingerprint := sha256.Sum256([]byte("image-request-1"))
	responseHash := sha256.Sum256([]byte("provider response"))
	candidate := `{"version":"ai_image_result_v1","task_id":81,"attempt_no":1,"outputs":[{"sort_order":1,"storage_provider":"cos","storage_key":"immutable/output.png","storage_url":"https://cdn.test/immutable/output.png","sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","mime_type":"image/png","width":2,"height":3,"size_bytes":10}]}`
	store := &imageFinalizationCaptureStore{facts: aigateway.FinalizationFacts{
		Run: aigateway.RunSnapshot{
			RunID: 51, UserID: 7, RequestID: "image-request-1", RequestFingerprint: fingerprint,
			PricingSnapshotJSON: testImagePricingSnapshotJSON(), BillingStatus: billing.BillingStatusHeld,
			BillingReason: billing.BillingReasonHeld, AgentID: 5, ModelID: "gpt-image-2",
		},
		Charge: aigateway.FinalizationCharge{ID: 61, RunID: 51, UserID: 7, HeldUnits: 10, HeldAuditMax: 10, Status: billing.ChargeStatusOpen},
		Hold: aigateway.FinalizationHold{
			ID: 71, WalletID: 81, RunID: 51, UserID: 7, HeldUnits: 10, HeldAuditMax: 10, Status: billing.HoldStatusActive,
		},
		Attempts: []aigateway.FinalizationAttempt{{
			ID: 91, RunID: 51, AttemptNo: 1, EvidenceKind: aigateway.AttemptEvidencePaid,
			State: billing.AttemptStateSucceeded, DispatchState: billing.DispatchStateDispatched,
			Usage:             infraai.UsageSnapshot{Status: infraai.UsageStatusUnavailable},
			ProviderRequestID: "provider-request-1", ResponseSHA256: responseHash,
		}},
		Trigger: aigateway.TriggerSuccess, CurrentAttemptID: 91,
		Candidate: aigateway.FinalizationCandidate{AttemptID: 91, JSON: candidate},
	}}

	err := aigateway.NewFinalizer(store, persistedSettlementPricer{}).Finalize(context.Background(), aigateway.FinalizeRequest{RunID: 51})

	if err != nil {
		t.Fatalf("finalize missing usage: %v", err)
	}
	decision := store.decision
	if decision.RunStatus != "failed" || decision.BillingStatus != billing.BillingStatusUnbilled ||
		decision.BillingReason != billing.BillingReasonUnbilledUsageIncomplete || decision.MoneyAction != aigateway.SettlementMoneyRelease ||
		decision.CandidateAction != aigateway.SettlementCandidateDiscard || decision.ActualUnits != 0 {
		t.Fatalf("missing usage decision=%+v", decision)
	}
}

func TestImageFilesFromCandidateRequiresExactTaskAndAttempt(t *testing.T) {
	candidate := imageResultCandidate{
		Version: imageResultCandidateVersion, TaskID: 81, AttemptNo: 1,
		Outputs: []imageCandidateOutput{{
			SortOrder: 1, StorageProvider: aiimage.StorageProviderCOS, StorageKey: "immutable/output.png",
			StorageURL: "https://cdn.test/immutable/output.png", SHA256: strings.Repeat("a", 64), MimeType: "image/png",
			Width: 2, Height: 3, SizeBytes: 10,
		}},
	}
	raw, err := jsonMarshalImageCandidate(candidate)
	if err != nil {
		t.Fatalf("marshal candidate: %v", err)
	}
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

	files, _, _, err := imageFilesFromCandidate(raw, 81, 1, now)
	if err != nil || len(files) != 1 || files[0].TaskID != 81 || files[0].StorageKey != candidate.Outputs[0].StorageKey {
		t.Fatalf("files=%#v err=%v", files, err)
	}
	if _, _, _, err := imageFilesFromCandidate(raw, 82, 1, now); err == nil {
		t.Fatal("candidate was rebound to another image task")
	}
	if _, _, _, err := imageFilesFromCandidate(raw, 81, 2, now); err == nil {
		t.Fatal("candidate was rebound to another provider attempt")
	}
}

type fakeImageObjectReader struct {
	result *storagecos.GetResult
	err    error
	gets   []storagecos.GetInput
}

func (reader *fakeImageObjectReader) Get(_ context.Context, input storagecos.GetInput) (*storagecos.GetResult, error) {
	reader.gets = append(reader.gets, input)
	if reader.result == nil {
		return nil, reader.err
	}
	copy := *reader.result
	copy.Body = append([]byte(nil), reader.result.Body...)
	return &copy, reader.err
}

func TestLoadPreparedImageAssetsVerifiesPersistedAttachmentDigest(t *testing.T) {
	body := []byte("immutable-image-bytes")
	digest := sha256.Sum256(body)
	reader := &fakeImageObjectReader{result: &storagecos.GetResult{Body: body, ContentType: "image/png"}}
	attachments := []aiimage.AttachmentSnapshot{{
		Role: aiimage.FileRoleInput, SortOrder: 1, StorageProvider: aiimage.StorageProviderCOS,
		StorageKey: "immutable/input.png", SHA256: hex.EncodeToString(digest[:]), MimeType: "image/png", SizeBytes: int64(len(body)),
	}}
	destination := imageObjectDestination{SecretID: "secret-id", SecretKey: "secret-key", Bucket: "bucket", Region: "region"}

	inputs, mask, err := loadPreparedImageAssets(context.Background(), reader, destination, attachments)

	if err != nil || mask != nil || len(inputs) != 1 || !bytes.Equal(inputs[0].Data, body) || inputs[0].SHA256 != attachments[0].SHA256 {
		t.Fatalf("inputs=%#v mask=%#v err=%v", inputs, mask, err)
	}
	reader.result.Body = []byte("changed-image-bytes")
	if _, _, err := loadPreparedImageAssets(context.Background(), reader, destination, attachments); err == nil {
		t.Fatal("changed attachment bytes passed immutable digest verification")
	}
}

type fakePaidImageExecutionStore struct {
	execution    *paidImageExecution
	attempt      replycommand.Attempt
	hasAttempt   bool
	loadCanceled bool
	unknownCalls int
}

func (store *fakePaidImageExecutionStore) LoadImageExecution(ctx context.Context, _ uint64) (*paidImageExecution, error) {
	store.loadCanceled = ctx.Err() != nil
	if store.execution == nil {
		return nil, nil
	}
	copy := *store.execution
	copy.Task = store.execution.Task
	return &copy, nil
}

func (store *fakePaidImageExecutionStore) LatestImageAttempt(context.Context, int64) (replycommand.Attempt, bool, error) {
	return store.attempt, store.hasAttempt, nil
}

func (store *fakePaidImageExecutionStore) MarkImageOutcomeUnknown(context.Context, uint64, int64, time.Time) error {
	store.unknownCalls++
	store.attempt.State = replycommand.AttemptOutcomeUnknown
	return nil
}

func (store *fakePaidImageExecutionStore) MarkImageFailure(context.Context, uint64, int64, string, string) error {
	return nil
}

type fakeImageFinalizer struct {
	store          *fakePaidImageExecutionStore
	calls          int
	terminalStatus string
}

func (finalizer *fakeImageFinalizer) Finalize(_ context.Context, _ aigateway.FinalizeRequest) error {
	finalizer.calls++
	status := finalizer.terminalStatus
	if status == "" {
		status = aiimage.StatusSuccess
	}
	finalizer.store.execution.Task.Status = status
	return nil
}

func TestPaidImageExecutorReplacementLeaseFinalizesDispatchedAttemptUnknownOnce(t *testing.T) {
	task := aiimage.ImageTask{
		ID: 82, UserID: 7, RunID: 52, RequestID: "image-request-2", RequestFingerprint: bytes.Repeat([]byte{2}, sha256.Size),
		RequestIdentityStatus: "replayable", AgentID: 5, ProviderIDSnapshot: 6, ModelIDSnapshot: "gpt-image-2", Status: aiimage.StatusRunning,
	}
	store := &fakePaidImageExecutionStore{
		execution: &paidImageExecution{Task: task}, attempt: replycommand.Attempt{ID: 92, RunID: 52, AttemptNo: 1, State: replycommand.AttemptDispatched}, hasAttempt: true,
	}
	finalizer := &fakeImageFinalizer{store: store, terminalStatus: aiimage.StatusFailed}
	executor := &paidImageTaskExecutor{store: store, finalizer: finalizer, now: func() time.Time { return time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC) }}
	lease := aiimage.TaskLease{Task: task, Owner: "worker-b", Token: 2, ExpiresAt: time.Now().Add(time.Minute)}
	ctx := aiimage.WithTaskLease(context.Background(), lease)

	status, err := executor.ExecuteImageTask(ctx, task.ID)
	if err != nil || status != aiimage.StatusFailed || store.unknownCalls != 1 || finalizer.calls != 1 {
		t.Fatalf("status=%q err=%v unknown=%d finalizer=%d", status, err, store.unknownCalls, finalizer.calls)
	}
	status, err = executor.ExecuteImageTask(ctx, task.ID)
	if err != nil || status != aiimage.StatusFailed || store.unknownCalls != 1 || finalizer.calls != 1 {
		t.Fatalf("replay status=%q err=%v unknown=%d finalizer=%d", status, err, store.unknownCalls, finalizer.calls)
	}
}

func TestPaidImageExecutorReplaysSucceededAttemptWithoutProviderCall(t *testing.T) {
	task := aiimage.ImageTask{
		ID: 81, UserID: 7, RunID: 51, RequestID: "image-request-1", RequestFingerprint: bytes.Repeat([]byte{1}, sha256.Size),
		RequestIdentityStatus: "replayable", AgentID: 5, ProviderIDSnapshot: 6, ModelIDSnapshot: "gpt-image-2", Status: aiimage.StatusRunning,
	}
	store := &fakePaidImageExecutionStore{
		execution:  &paidImageExecution{Task: task},
		attempt:    replycommand.Attempt{ID: 91, RunID: 51, AttemptNo: 1, State: replycommand.AttemptSucceeded},
		hasAttempt: true,
	}
	finalizer := &fakeImageFinalizer{store: store}
	dispatchCalls := 0
	executor := &paidImageTaskExecutor{
		store: store, finalizer: finalizer,
		dispatch: func(context.Context, paidImageExecution, uint32, bool) error {
			dispatchCalls++
			return nil
		},
	}
	lease := aiimage.TaskLease{Task: task, Owner: "worker-a", Token: 1, ExpiresAt: time.Now().Add(time.Minute)}
	ctx := aiimage.WithTaskLease(context.Background(), lease)

	status, err := executor.ExecuteImageTask(ctx, task.ID)

	if err != nil || status != aiimage.StatusSuccess {
		t.Fatalf("status=%q err=%v", status, err)
	}
	if finalizer.calls != 1 || dispatchCalls != 0 || store.loadCanceled {
		t.Fatalf("finalizer=%d dispatch=%d loadCanceled=%v", finalizer.calls, dispatchCalls, store.loadCanceled)
	}
}

func TestImageUnbilledTerminalReplayRequiresReleasedHold(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	run := airun.Run{ID: 51, UserID: 7, BillingStatus: string(billing.BillingStatusUnbilled)}
	charge := billing.UsageCharge{ID: 61, RunID: 51, UserID: 7, Status: billing.ChargeStatusUnbilled, FinalizedAt: &now}
	task := aiimage.ImageTask{ID: 81, RunID: 51, Status: aiimage.StatusFailed, FinishedAt: &now}

	if err := validateImageFinalizationReplay(run, charge, nil, nil, task, nil, nil); err == nil {
		t.Fatal("unbilled terminal replay without a released Hold was accepted")
	}
	hold := &walletmodule.Hold{ID: 71, WalletID: 72, RunID: 51, UserID: 7, Status: walletmodule.HoldReleased}
	wallet := &walletmodule.Wallet{ID: 72, UserID: 7}
	if err := validateImageFinalizationReplay(run, charge, wallet, hold, task, nil, nil); err != nil {
		t.Fatalf("valid unbilled replay: %v", err)
	}
}

func jsonMarshalImageCandidate(candidate imageResultCandidate) (string, error) {
	raw, err := json.Marshal(candidate)
	return string(raw), err
}

func testRuntimePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			value.Set(x, y, color.RGBA{R: 0x44, G: 0x88, B: 0xcc, A: 0xff})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, value); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buffer.Bytes()
}

func testImagePricingSnapshotJSON() string {
	return `{"version":"official_numeric_parity_v2","billable":true,"catalog_vendor":"openai","transport_engine":"openai","requested_model_id":"gpt-image-2","canonical_model_id":"gpt-image-2","catalog_max_output_tokens":32768,"effective_max_output_tokens":32768,"multiplier_ppm":1000000,"source_url":"https://developers.openai.com/api/docs/pricing#image-generation","retrieved_at":"2026-07-26","rates":[{"category":"input","unit":"token","tier_key":"","price_units":500000000,"unit_scale":1000000},{"category":"output","unit":"token","tier_key":"","price_units":3000000000,"unit_scale":1000000}]}`
}
