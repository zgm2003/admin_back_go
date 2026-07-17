package runtime

import (
	"context"
	"errors"
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
	aiaudio "admin_back_go/internal/module/ai/audio"
	aichat "admin_back_go/internal/module/ai/chat"
	aiimage "admin_back_go/internal/module/ai/image"
	aiprovider "admin_back_go/internal/module/ai/provider"
	aitool "admin_back_go/internal/module/ai/tool"
	aivideo "admin_back_go/internal/module/ai/video"
	"admin_back_go/internal/module/mail"
	"admin_back_go/internal/module/sms"
)

type Providers struct {
	Secretbox secretbox.Box

	MailSender mail.Sender
	SMSSender  sms.Sender

	AIConnectionTester aiprovider.ProviderTester
	AIChatFactory      aichat.EngineFactory
	AIImageFactory     aiimage.ImageEngineFactory
	AIToolFactory      aitool.EngineFactory
	AIVideoFactory     aivideo.EngineFactory
	AIAudioFactory     aiaudio.EngineFactory

	ObjectReader     storagecos.ObjectReader
	ObjectWriter     storagecos.ObjectWriter
	CredentialSigner storagecos.CredentialSigner

	PaymentGateway      paymentcore.Gateway
	PaymentCertResolver paymentcore.CertPathResolver
	PaymentCertStore    paymentcore.LocalCertStore
}

func BuildProviders(cfg config.Config, keys *secretkey.KeyRing) (Providers, error) {
	if keys == nil || len(keys.SecretboxKey()) == 0 {
		return Providers{}, errors.New("runtime providers require a key ring")
	}
	box := secretbox.New(keys.SecretboxKey())
	return Providers{
		Secretbox:           box,
		MailSender:          newMailSender(),
		SMSSender:           newSMSSender(),
		AIConnectionTester:  aiConnectionTester{},
		AIChatFactory:       aiChatEngineFactory{streamIdleTimeout: positiveProviderDuration(cfg.AI.ChatStreamIdleTimeout, config.DefaultAIChatStreamIdleTimeout)},
		AIImageFactory:      aiImageEngineFactory{},
		AIToolFactory:       aiToolEngineFactory{},
		AIVideoFactory:      aiVideoEngineFactory{},
		AIAudioFactory:      aiAudioEngineFactory{},
		ObjectReader:        storagecos.NewObjectReader(storagecos.ObjectReaderConfig{Enabled: true}),
		ObjectWriter:        storagecos.NewObjectWriter(storagecos.ObjectWriterConfig{Enabled: true}),
		CredentialSigner:    storagecos.NewSigner(storagecos.Config{Enabled: true}),
		PaymentGateway:      payalipay.NewPlatformGateway(payalipay.NewGopayGateway()),
		PaymentCertResolver: paymentcore.CertPathResolver{CertBaseDir: cfg.Payment.CertBaseDir, WorkingDir: "."},
		PaymentCertStore:    paymentcore.LocalCertStore{BaseDir: cfg.Payment.CertBaseDir},
	}, nil
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

type aiConnectionTester struct{}

func (aiConnectionTester) TestConnection(ctx context.Context, input infraai.TestConnectionInput) (*infraai.TestConnectionResult, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL: input.BaseURL,
			APIKey:  input.APIKey,
			Timeout: time.Duration(input.TimeoutMs) * time.Millisecond,
		}).TestConnection(ctx, input)
	default:
		return nil, infraai.ErrInvalidConfig
	}
}

type aiChatEngineFactory struct {
	streamIdleTimeout time.Duration
}

func (factory aiChatEngineFactory) NewEngine(_ context.Context, input aichat.EngineConfig) (infraai.Engine, error) {
	switch input.EngineType {
	case infraai.EngineTypeOpenAI:
		return openaicompat.New(openaicompat.Config{
			BaseURL:           input.BaseURL,
			APIKey:            input.APIKey,
			Timeout:           30 * time.Second,
			StreamIdleTimeout: factory.streamIdleTimeout,
		}), nil
	default:
		return nil, infraai.ErrInvalidConfig
	}
}

type aiImageEngineFactory struct{}

func (aiImageEngineFactory) NewImageEngine(input aiimage.ImageEngineConfig) infraai.ImageEngine {
	if infraai.EngineType(input.EngineType) != infraai.EngineTypeOpenAI {
		return nil
	}
	return imagecompat.New(imagecompat.Config{BaseURL: input.BaseURL, APIKey: input.APIKey, Timeout: input.Timeout})
}

type aiToolEngineFactory struct{}

func (aiToolEngineFactory) NewEngine(_ context.Context, input aitool.EngineConfig) (infraai.Engine, error) {
	if input.EngineType != infraai.EngineTypeOpenAI {
		return nil, infraai.ErrInvalidConfig
	}
	return openaicompat.New(openaicompat.Config{BaseURL: input.BaseURL, APIKey: input.APIKey, Timeout: 30 * time.Second}), nil
}

type aiVideoEngineFactory struct{}

func (aiVideoEngineFactory) NewVideoEngine(_ context.Context, input aivideo.EngineConfig) (infraai.VideoEngine, error) {
	if input.EngineType != infraai.EngineTypeOpenAI {
		return nil, infraai.ErrInvalidConfig
	}
	return openaicompat.New(openaicompat.Config{BaseURL: input.BaseURL, APIKey: input.APIKey, Timeout: 10 * time.Minute}), nil
}

type aiAudioEngineFactory struct{}

func (aiAudioEngineFactory) NewAudioEngine(_ context.Context, input aiaudio.EngineConfig) (infraai.AudioEngine, error) {
	if input.EngineType != infraai.EngineTypeOpenAI {
		return nil, infraai.ErrInvalidConfig
	}
	return openaicompat.New(openaicompat.Config{BaseURL: input.BaseURL, APIKey: input.APIKey, Timeout: 2 * time.Minute}), nil
}

func positiveProviderDuration(value time.Duration, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}
