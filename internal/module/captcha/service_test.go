package captcha

import (
	"context"
	"errors"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
)

type fakeEngine struct {
	challenge *GeneratedChallenge
	err       error
}

func (f fakeEngine) Generate() (*GeneratedChallenge, error) {
	return f.challenge, f.err
}

type fakeStore struct {
	setID     string
	setSecret ChallengeSecret
	setTTL    time.Duration
	takeID    string
	secret    *ChallengeSecret
	setErr    error
	takeErr   error
}

func (f *fakeStore) Set(ctx context.Context, id string, secret ChallengeSecret, ttl time.Duration) error {
	f.setID = id
	f.setSecret = secret
	f.setTTL = ttl
	return f.setErr
}

func (f *fakeStore) Take(ctx context.Context, id string) (*ChallengeSecret, error) {
	f.takeID = id
	return f.secret, f.takeErr
}

func TestServiceGenerateStoresSecretAndReturnsPublicSlidePayload(t *testing.T) {
	store := &fakeStore{}
	service := NewService(
		fakeEngine{challenge: &GeneratedChallenge{
			MasterImage: "data:image/jpeg;base64,master",
			TileImage:   "data:image/png;base64,tile",
			TileX:       7,
			TileY:       53,
			TileWidth:   62,
			TileHeight:  62,
			ImageWidth:  300,
			ImageHeight: 220,
			Answer:      Answer{X: 131, Y: 53},
		}},
		store,
		WithIDGenerator(func() (string, error) { return "captcha-id", nil }),
		WithTTL(90*time.Second),
	)

	result, appErr := service.Generate(context.Background())

	if appErr != nil {
		t.Fatalf("expected generate to succeed, got %v", appErr)
	}
	if result.CaptchaID != "captcha-id" || result.CaptchaType != TypeSlide {
		t.Fatalf("unexpected captcha identity: %#v", result)
	}
	if result.MasterImage != "data:image/jpeg;base64,master" || result.TileImage != "data:image/png;base64,tile" {
		t.Fatalf("unexpected images: %#v", result)
	}
	if result.TileX != 7 || result.TileY != 53 || result.TileWidth != 62 || result.TileHeight != 62 ||
		result.ImageWidth != 300 || result.ImageHeight != 220 || result.ExpiresIn != 90 {
		t.Fatalf("unexpected public payload: %#v", result)
	}
	if store.setID != "captcha-id" || store.setSecret.Answer.X != 131 || store.setSecret.Answer.Y != 53 ||
		store.setTTL != 90*time.Second {
		t.Fatalf("unexpected stored secret: id=%q secret=%#v ttl=%s", store.setID, store.setSecret, store.setTTL)
	}
}

func TestServiceVerifyConsumesAndAcceptsValidAnswer(t *testing.T) {
	store := &fakeStore{secret: &ChallengeSecret{Answer: Answer{X: 120, Y: 80}}}
	service := NewService(fakeEngine{}, store, WithPadding(3))

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 122, Y: 81},
	})

	if appErr != nil {
		t.Fatalf("expected verify to succeed, got %v", appErr)
	}
	if store.takeID != "captcha-id" {
		t.Fatalf("expected captcha to be consumed, got %q", store.takeID)
	}
}

func TestServiceVerifyRejectsMissingOrReusedChallenge(t *testing.T) {
	store := &fakeStore{}
	service := NewService(fakeEngine{}, store)

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 120, Y: 80},
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "验证码错误或已过期" {
		t.Fatalf("expected missing captcha rejection, got %#v", appErr)
	}
	if store.takeID != "captcha-id" {
		t.Fatalf("expected captcha lookup to run, got %q", store.takeID)
	}
}

func TestServiceVerifyRejectsWrongAnswer(t *testing.T) {
	store := &fakeStore{secret: &ChallengeSecret{Answer: Answer{X: 120, Y: 80}}}
	service := NewService(fakeEngine{}, store, WithPadding(3))

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 40, Y: 80},
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "验证码错误或已过期" {
		t.Fatalf("expected wrong captcha rejection, got %#v", appErr)
	}
}

func TestServiceVerifyErrorsCarryMessageIDs(t *testing.T) {
	service := NewService(fakeEngine{}, &fakeStore{})

	appErr := service.Verify(context.Background(), VerifyInput{})

	if appErr == nil {
		t.Fatalf("expected captcha validation error")
	}
	if appErr.MessageID != "captcha.required" {
		t.Fatalf("expected captcha.required message id, got %#v", appErr)
	}
	if appErr.Message != "请完成验证码" {
		t.Fatalf("fallback message changed: %#v", appErr)
	}
}

func TestServiceVerifyFailsClosedWhenStoreErrors(t *testing.T) {
	service := NewService(fakeEngine{}, &fakeStore{takeErr: errors.New("redis down")})

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 120, Y: 80},
	})

	if appErr == nil || appErr.Code != apperror.CodeInternal || appErr.Message != "验证码校验失败" {
		t.Fatalf("expected internal captcha error, got %#v", appErr)
	}
}
