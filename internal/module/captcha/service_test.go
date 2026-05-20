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
	setTTLs   []time.Duration
	takeID    string
	secret    *ChallengeSecret
	secrets   []*ChallengeSecret
	setErr    error
	takeErr   error
}

func (f *fakeStore) Set(ctx context.Context, id string, secret ChallengeSecret, ttl time.Duration) error {
	f.setID = id
	f.setSecret = secret
	f.setTTL = ttl
	f.setTTLs = append(f.setTTLs, ttl)
	return f.setErr
}

func (f *fakeStore) Take(ctx context.Context, id string) (*ChallengeSecret, error) {
	f.takeID = id
	if len(f.secrets) > 0 {
		secret := f.secrets[0]
		f.secrets = f.secrets[1:]
		return secret, f.takeErr
	}
	return f.secret, f.takeErr
}

type fakeCaptchaPolicy struct {
	ttlValues     []time.Duration
	paddingValues []int
	ttlErr        *apperror.Error
	paddingErr    *apperror.Error
	ttlCalls      int
	paddingCalls  int
}

func (f *fakeCaptchaPolicy) TTL(ctx context.Context) (time.Duration, *apperror.Error) {
	f.ttlCalls++
	if f.ttlErr != nil {
		return 0, f.ttlErr
	}
	if len(f.ttlValues) == 0 {
		return 2 * time.Minute, nil
	}
	ttl := f.ttlValues[0]
	if len(f.ttlValues) > 1 {
		f.ttlValues = f.ttlValues[1:]
	}
	return ttl, nil
}

func (f *fakeCaptchaPolicy) SlidePadding(ctx context.Context) (int, *apperror.Error) {
	f.paddingCalls++
	if f.paddingErr != nil {
		return 0, f.paddingErr
	}
	if len(f.paddingValues) == 0 {
		return 10, nil
	}
	padding := f.paddingValues[0]
	if len(f.paddingValues) > 1 {
		f.paddingValues = f.paddingValues[1:]
	}
	return padding, nil
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
		&fakeCaptchaPolicy{ttlValues: []time.Duration{90 * time.Second}},
		WithIDGenerator(func() (string, error) { return "captcha-id", nil }),
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

func TestServiceGenerateReadsTTLFromPolicyEveryCall(t *testing.T) {
	store := &fakeStore{}
	policy := &fakeCaptchaPolicy{ttlValues: []time.Duration{30 * time.Second, 75 * time.Second}}
	nextID := 0
	service := NewService(
		fakeEngine{challenge: &GeneratedChallenge{Answer: Answer{X: 131, Y: 53}}},
		store,
		policy,
		WithIDGenerator(func() (string, error) {
			nextID++
			return "captcha-id", nil
		}),
	)

	first, appErr := service.Generate(context.Background())
	if appErr != nil {
		t.Fatalf("expected first generate to succeed, got %v", appErr)
	}
	second, appErr := service.Generate(context.Background())
	if appErr != nil {
		t.Fatalf("expected second generate to succeed, got %v", appErr)
	}

	if first.ExpiresIn != 30 || second.ExpiresIn != 75 {
		t.Fatalf("expected dynamic expires_in values, got first=%d second=%d", first.ExpiresIn, second.ExpiresIn)
	}
	if len(store.setTTLs) != 2 || store.setTTLs[0] != 30*time.Second || store.setTTLs[1] != 75*time.Second {
		t.Fatalf("expected dynamic store TTLs, got %#v", store.setTTLs)
	}
	if policy.ttlCalls != 2 {
		t.Fatalf("expected TTL policy to be read per Generate call, got %d", policy.ttlCalls)
	}
}

func TestServiceVerifyConsumesAndAcceptsValidAnswer(t *testing.T) {
	store := &fakeStore{secret: &ChallengeSecret{Answer: Answer{X: 120, Y: 80}}}
	service := NewService(fakeEngine{}, store, &fakeCaptchaPolicy{paddingValues: []int{3}})

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

func TestServiceVerifyReadsPaddingFromPolicyEveryCall(t *testing.T) {
	store := &fakeStore{secrets: []*ChallengeSecret{
		{Answer: Answer{X: 120, Y: 80}},
		{Answer: Answer{X: 120, Y: 80}},
	}}
	policy := &fakeCaptchaPolicy{paddingValues: []int{3, 15}}
	service := NewService(fakeEngine{}, store, policy)

	firstErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id-1",
		Answer: &Answer{X: 130, Y: 80},
	})
	secondErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id-2",
		Answer: &Answer{X: 130, Y: 80},
	})

	if firstErr == nil || firstErr.Code != apperror.CodeBadRequest {
		t.Fatalf("expected first verify to reject with small padding, got %#v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("expected second verify to accept with updated padding, got %v", secondErr)
	}
	if policy.paddingCalls != 2 {
		t.Fatalf("expected padding policy to be read per Verify call, got %d", policy.paddingCalls)
	}
}

func TestServiceVerifyRejectsMissingOrReusedChallenge(t *testing.T) {
	store := &fakeStore{}
	service := NewService(fakeEngine{}, store, &fakeCaptchaPolicy{})

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
	service := NewService(fakeEngine{}, store, &fakeCaptchaPolicy{paddingValues: []int{3}})

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 40, Y: 80},
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "验证码错误或已过期" {
		t.Fatalf("expected wrong captcha rejection, got %#v", appErr)
	}
}

func TestServiceVerifyErrorsCarryMessageIDs(t *testing.T) {
	service := NewService(fakeEngine{}, &fakeStore{}, &fakeCaptchaPolicy{})

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
	service := NewService(fakeEngine{}, &fakeStore{takeErr: errors.New("redis down")}, &fakeCaptchaPolicy{})

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 120, Y: 80},
	})

	if appErr == nil || appErr.Code != apperror.CodeInternal || appErr.Message != "验证码校验失败" {
		t.Fatalf("expected internal captcha error, got %#v", appErr)
	}
}

func TestServiceReturnsServiceMissingWhenPolicyMissing(t *testing.T) {
	service := NewService(fakeEngine{}, &fakeStore{}, nil)

	_, generateErr := service.Generate(context.Background())
	verifyErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 120, Y: 80},
	})

	if generateErr == nil || generateErr.MessageID != "captcha.service_missing" {
		t.Fatalf("expected generate service missing, got %#v", generateErr)
	}
	if verifyErr == nil || verifyErr.MessageID != "captcha.service_missing" {
		t.Fatalf("expected verify service missing, got %#v", verifyErr)
	}
}
