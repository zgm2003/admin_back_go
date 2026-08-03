package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"admin_back_go/internal/config"
	infraai "admin_back_go/internal/infra/ai"
	"admin_back_go/internal/infra/ai/imagecompat"
	"admin_back_go/internal/infra/ai/openaicompat"
	inframail "admin_back_go/internal/infra/mail/tencentcloudses"
	paymentcore "admin_back_go/internal/infra/payment"
	payalipay "admin_back_go/internal/infra/payment/alipay"
	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
	infrasms "admin_back_go/internal/infra/sms/tencentcloudsms"
	storagecos "admin_back_go/internal/infra/storage/cos"
	aichat "admin_back_go/internal/module/ai/chat"
	aiimage "admin_back_go/internal/module/ai/image"
	aiprovider "admin_back_go/internal/module/ai/provider"
	aitool "admin_back_go/internal/module/ai/tool"
	"admin_back_go/internal/module/mail"
	"admin_back_go/internal/module/sms"
	"admin_back_go/internal/telemetry"
)

type Providers struct {
	Secretbox         secretbox.Box
	MailDiagnosticBox secretbox.VersionedBox

	MailSender mail.Sender
	SMSSender  sms.Sender

	AIConnectionTester      aiprovider.ProviderTester
	AIChatFactory           aichat.EngineFactory
	AIEmbeddingFactory      infraai.EmbeddingFactory
	AIImageFactory          aiimage.ImageEngineFactory
	AIToolFactory           aitool.EngineFactory
	AITransportCapabilities infraai.TransportCapabilityResolver

	ObjectReader     storagecos.ObjectReader
	ObjectWriter     storagecos.ObjectWriter
	CredentialSigner storagecos.CredentialSigner

	PaymentGateway      paymentcore.Gateway
	PaymentCertResolver paymentcore.CertPathResolver
	PaymentCertStore    paymentcore.LocalCertStore
}

func BuildProviders(cfg config.Config, keys *secretkey.KeyRing, logger *slog.Logger, recorders ...telemetry.Recorder) (Providers, error) {
	if keys == nil || len(keys.SecretboxKey()) == 0 {
		return Providers{}, errors.New("runtime providers require a key ring")
	}
	if logger == nil {
		logger = slog.Default()
	}
	recorder := telemetry.Noop()
	if len(recorders) > 0 && recorders[0] != nil {
		recorder = recorders[0]
	}
	box := secretbox.New(keys.SecretboxKey())
	diagnosticBox, err := secretbox.NewVersioned(keys.MailDiagnosticKeyID(), keys.MailDiagnosticDecryptionKeys())
	if err != nil {
		return Providers{}, fmt.Errorf("build mail diagnostic box: %w", err)
	}
	return Providers{
		Secretbox:          box,
		MailDiagnosticBox:  diagnosticBox,
		MailSender:         newMailSender(),
		SMSSender:          newSMSSender(),
		AIConnectionTester: aiConnectionTester{logger: logger, recorder: recorder},
		AIChatFactory:      aiChatEngineFactory{logger: logger, streamIdleTimeout: positiveProviderDuration(cfg.AI.ChatStreamIdleTimeout, config.DefaultAIChatStreamIdleTimeout), recorder: recorder},
		AIEmbeddingFactory: aiEmbeddingFactory{},
		AIImageFactory:     aiImageEngineFactory{recorder: recorder},
		AIToolFactory:      aiToolEngineFactory{logger: logger, recorder: recorder},
		AITransportCapabilities: infraai.TransportCapabilityResolverFunc(func(engineType infraai.EngineType) (infraai.CapabilityMetadata, bool) {
			return infraai.DefaultTransportCapabilities(engineType)
		}),
		ObjectReader:        storagecos.NewObjectReader(storagecos.ObjectReaderConfig{Enabled: true}),
		ObjectWriter:        storagecos.NewObjectWriter(storagecos.ObjectWriterConfig{Enabled: true}),
		CredentialSigner:    storagecos.NewSigner(storagecos.Config{Enabled: true}),
		PaymentGateway:      payalipay.NewPlatformGateway(payalipay.NewGopayGateway()),
		PaymentCertResolver: paymentcore.CertPathResolver{CertBaseDir: cfg.Payment.CertBaseDir, WorkingDir: "."},
		PaymentCertStore:    paymentcore.LocalCertStore{BaseDir: cfg.Payment.CertBaseDir},
	}, nil
}

type aiEmbeddingFactory struct{}

func (aiEmbeddingFactory) NewEmbeddingClient(_ context.Context, input infraai.EmbeddingClientConfig) (infraai.EmbeddingClient, error) {
	if input.ModelKind != string(aiprovider.ModelKindEmbedding) {
		return nil, fmt.Errorf("%w: provider model kind must be embedding", infraai.ErrEmbeddingFailed)
	}
	if err := input.Capabilities.Validate(); err != nil {
		return nil, err
	}
	if input.EngineType != infraai.EngineTypeOpenAI {
		return nil, fmt.Errorf("%w: unsupported embedding engine type", infraai.ErrEmbeddingFailed)
	}
	return openaicompat.NewEmbeddingClient(openaicompat.Config{BaseURL: input.BaseURL, APIKey: input.APIKey}, input.ModelID, input.Capabilities)
}

func newMailSender() mail.Sender {
	client := inframail.New(10 * time.Second)
	return mail.SenderFunc(func(ctx context.Context, input mail.SendInput) (mail.SendResult, error) {
		result, err := client.Send(ctx, inframail.SendInput{
			SecretID:     input.SecretID,
			SecretKey:    input.SecretKey,
			Region:       input.Region,
			Endpoint:     input.Endpoint,
			FromEmail:    input.FromEmail,
			FromName:     input.FromName,
			ReplyTo:      input.ReplyTo,
			ToEmail:      input.ToEmail,
			Subject:      input.Subject,
			TemplateID:   input.TemplateID,
			TemplateData: input.TemplateData,
		})
		if err != nil {
			return mail.SendResult{}, err
		}
		return mail.SendResult{RequestID: result.RequestID, MessageID: result.MessageID}, nil
	})
}

func newSMSSender() sms.Sender {
	client := infrasms.New(10 * time.Second)
	return sms.SenderFunc(func(ctx context.Context, input sms.SendInput) (sms.SendResult, error) {
		result, err := client.Send(ctx, infrasms.SendInput{
			SecretID:         input.SecretID,
			SecretKey:        input.SecretKey,
			Region:           input.Region,
			Endpoint:         input.Endpoint,
			SmsSdkAppID:      input.SmsSdkAppID,
			SignName:         input.SignName,
			TemplateID:       input.TemplateID,
			PhoneNumber:      input.PhoneNumber,
			TemplateParamSet: input.TemplateParamSet,
		})
		return sms.SendResult{RequestID: result.RequestID, SerialNo: result.SerialNo, Fee: result.Fee}, err
	})
}

type aiConnectionTester struct {
	logger   *slog.Logger
	recorder telemetry.Recorder
}

func (tester aiConnectionTester) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		engine := openaicompat.New(openaicompat.Config{
			BaseURL: input.BaseURL,
			APIKey:  input.APIKey,
			Timeout: time.Duration(input.TimeoutMs) * time.Millisecond,
			Logger:  tester.logger,
		})
		return infraai.InstrumentEngine(string(input.EngineType), "connection", engine, tester.recorder).TestConnection(ctx, input)
	default:
		return nil, infraai.ErrInvalidConfig
	}
}

type aiChatEngineFactory struct {
	logger            *slog.Logger
	streamIdleTimeout time.Duration
	recorder          telemetry.Recorder
}

func (factory aiChatEngineFactory) NewEngine(_ context.Context, input aichat.EngineConfig) (infraai.Engine, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		engine := openaicompat.New(openaicompat.Config{
			BaseURL:           input.BaseURL,
			APIKey:            input.APIKey,
			Timeout:           30 * time.Second,
			StreamIdleTimeout: factory.streamIdleTimeout,
			APIProtocol:       input.APIProtocol,
			FileOpener:        input.FileOpener,
			Logger:            factory.logger,
		})
		return infraai.InstrumentEngine(string(input.EngineType), "chat", engine, factory.recorder), nil
	default:
		return nil, infraai.ErrInvalidConfig
	}
}

type aiImageEngineFactory struct {
	recorder telemetry.Recorder
}

func (factory aiImageEngineFactory) NewImageEngine(input aiimage.ImageEngineConfig) infraai.ImageEngine {
	if infraai.EngineType(input.EngineType) != infraai.EngineTypeOpenAI {
		return nil
	}
	engine := imagecompat.New(imagecompat.Config{BaseURL: input.BaseURL, APIKey: input.APIKey, Timeout: input.Timeout})
	return infraai.InstrumentImageEngine(input.EngineType, engine, factory.recorder)
}

type aiToolEngineFactory struct {
	logger   *slog.Logger
	recorder telemetry.Recorder
}

func (factory aiToolEngineFactory) NewEngine(_ context.Context, input aitool.EngineConfig) (infraai.Engine, error) {
	if input.EngineType != infraai.EngineTypeOpenAI {
		return nil, infraai.ErrInvalidConfig
	}
	engine := openaicompat.New(openaicompat.Config{BaseURL: input.BaseURL, APIKey: input.APIKey, Timeout: 30 * time.Second, APIProtocol: input.APIProtocol, Logger: factory.logger})
	return infraai.InstrumentEngine(string(input.EngineType), "tool", engine, factory.recorder), nil
}

func positiveProviderDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
