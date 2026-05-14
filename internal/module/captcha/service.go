package captcha

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"admin_back_go/internal/apperror"

	"github.com/wenlng/go-captcha/v2/slide"
)

const (
	defaultTTL     = 2 * time.Minute
	defaultPadding = 10
)

// IDGenerator creates challenge identifiers.
type IDGenerator func() (string, error)

// Service owns CAPTCHA generation and verification.
type Service struct {
	engine      Engine
	store       Store
	ttl         time.Duration
	padding     int
	idGenerator IDGenerator
}

// Option customizes Service.
type Option func(*Service)

// WithTTL sets challenge lifetime.
func WithTTL(ttl time.Duration) Option {
	return func(service *Service) {
		if ttl > 0 {
			service.ttl = ttl
		}
	}
}

// WithPadding sets the tolerated slide-answer offset.
func WithPadding(padding int) Option {
	return func(service *Service) {
		if padding >= 0 {
			service.padding = padding
		}
	}
}

// WithIDGenerator injects a challenge ID generator for tests.
func WithIDGenerator(generator IDGenerator) Option {
	return func(service *Service) {
		if generator != nil {
			service.idGenerator = generator
		}
	}
}

// NewService creates a CAPTCHA service.
func NewService(engine Engine, store Store, opts ...Option) *Service {
	service := &Service{
		engine:      engine,
		store:       store,
		ttl:         defaultTTL,
		padding:     defaultPadding,
		idGenerator: makeID,
	}
	for _, opt := range opts {
		opt(service)
	}
	return service
}

// Generate creates a new slide CAPTCHA and stores its answer.
func (s *Service) Generate(ctx context.Context) (*ChallengeResponse, *apperror.Error) {
	if s == nil || s.engine == nil || s.store == nil {
		return nil, apperror.InternalKey("captcha.service_missing", nil, "验证码服务未配置")
	}

	generated, err := s.engine.Generate()
	if err != nil {
		return nil, apperror.InternalKey("captcha.generate_failed", nil, "验证码生成失败")
	}
	id, err := s.idGenerator()
	if err != nil {
		return nil, apperror.InternalKey("captcha.generate_failed", nil, "验证码生成失败")
	}

	if err := s.store.Set(ctx, id, ChallengeSecret{Answer: generated.Answer}, s.ttl); err != nil {
		return nil, apperror.InternalKey("captcha.generate_failed", nil, "验证码生成失败")
	}

	return &ChallengeResponse{
		CaptchaID:   id,
		CaptchaType: TypeSlide,
		MasterImage: generated.MasterImage,
		TileImage:   generated.TileImage,
		TileX:       generated.TileX,
		TileY:       generated.TileY,
		TileWidth:   generated.TileWidth,
		TileHeight:  generated.TileHeight,
		ImageWidth:  generated.ImageWidth,
		ImageHeight: generated.ImageHeight,
		ExpiresIn:   int(s.ttl.Seconds()),
	}, nil
}

// Verify consumes a challenge and validates the submitted slide answer.
func (s *Service) Verify(ctx context.Context, input VerifyInput) *apperror.Error {
	if s == nil || s.store == nil {
		return apperror.InternalKey("captcha.service_missing", nil, "验证码服务未配置")
	}
	id := strings.TrimSpace(input.ID)
	if id == "" || input.Answer == nil {
		return apperror.BadRequestKey("captcha.required", nil, "请完成验证码")
	}

	secret, err := s.store.Take(ctx, id)
	if err != nil {
		return apperror.InternalKey("captcha.verify_failed", nil, "验证码校验失败")
	}
	if secret == nil {
		return apperror.BadRequestKey("captcha.invalid_or_expired", nil, "验证码错误或已过期")
	}
	if !slide.Validate(input.Answer.X, input.Answer.Y, secret.Answer.X, secret.Answer.Y, s.padding) {
		return apperror.BadRequestKey("captcha.invalid_or_expired", nil, "验证码错误或已过期")
	}
	return nil
}

func makeID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
