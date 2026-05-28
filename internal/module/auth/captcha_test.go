package auth

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/enum"
	"admin_back_go/internal/module/systemsetting"
	sharedsetting "admin_back_go/internal/shared/setting"
)

type fakeCaptchaPolicyRepository struct {
	rows map[string]*systemsetting.Setting
	err  error
	keys []string
}

func (f *fakeCaptchaPolicyRepository) SettingByKey(ctx context.Context, key string) (*systemsetting.Setting, error) {
	f.keys = append(f.keys, key)
	if f.err != nil {
		return nil, f.err
	}
	return f.rows[key], nil
}

func TestSystemSettingCaptchaPolicyProviderReadsTTLAndPadding(t *testing.T) {
	repo := &fakeCaptchaPolicyRepository{rows: map[string]*systemsetting.Setting{
		sharedsetting.AuthCaptchaTTLKey: {
			SettingKey:   sharedsetting.AuthCaptchaTTLKey,
			SettingValue: "5",
			ValueType:    enum.SystemSettingValueNumber,
			Status:       enum.CommonYes,
			IsDel:        enum.CommonNo,
		},
		CaptchaSlidePaddingSettingKey: {
			SettingKey:   CaptchaSlidePaddingSettingKey,
			SettingValue: "12",
			ValueType:    enum.SystemSettingValueNumber,
			Status:       enum.CommonYes,
			IsDel:        enum.CommonNo,
		},
	}}
	provider := NewSystemSettingCaptchaPolicyProvider(repo)

	ttl, appErr := provider.TTL(context.Background())
	if appErr != nil {
		t.Fatalf("expected TTL read to succeed, got %v", appErr)
	}
	padding, appErr := provider.SlidePadding(context.Background())
	if appErr != nil {
		t.Fatalf("expected slide padding read to succeed, got %v", appErr)
	}

	if ttl != 5*time.Minute {
		t.Fatalf("expected ttl 5m from system setting, got %s", ttl)
	}
	if padding != 12 {
		t.Fatalf("expected padding 12 from system setting, got %d", padding)
	}
	wantKeys := []string{sharedsetting.AuthCaptchaTTLKey, CaptchaSlidePaddingSettingKey}
	if !reflect.DeepEqual(repo.keys, wantKeys) {
		t.Fatalf("expected SettingByKey keys %#v, got %#v", wantKeys, repo.keys)
	}
}

func TestSystemSettingCaptchaPolicyProviderAllowsZeroSlidePadding(t *testing.T) {
	provider := NewSystemSettingCaptchaPolicyProvider(&fakeCaptchaPolicyRepository{rows: map[string]*systemsetting.Setting{
		CaptchaSlidePaddingSettingKey: validCaptchaSetting(CaptchaSlidePaddingSettingKey, "0", enum.CommonYes),
	}})

	padding, appErr := provider.SlidePadding(context.Background())

	if appErr != nil {
		t.Fatalf("expected zero slide padding to stay valid, got %v", appErr)
	}
	if padding != 0 {
		t.Fatalf("expected zero slide padding, got %d", padding)
	}
}

func TestSystemSettingCaptchaPolicyProviderFailsClosedForInvalidSettings(t *testing.T) {
	cases := []struct {
		name string
		rows map[string]*systemsetting.Setting
	}{
		{name: "missing", rows: map[string]*systemsetting.Setting{}},
		{name: "disabled", rows: map[string]*systemsetting.Setting{
			sharedsetting.AuthCaptchaTTLKey: validCaptchaSetting(sharedsetting.AuthCaptchaTTLKey, "2", enum.CommonNo),
		}},
		{name: "wrong type", rows: map[string]*systemsetting.Setting{
			sharedsetting.AuthCaptchaTTLKey: {
				SettingKey:   sharedsetting.AuthCaptchaTTLKey,
				SettingValue: "2",
				ValueType:    enum.SystemSettingValueString,
				Status:       enum.CommonYes,
				IsDel:        enum.CommonNo,
			},
		}},
		{name: "not integer", rows: map[string]*systemsetting.Setting{
			sharedsetting.AuthCaptchaTTLKey: validCaptchaSetting(sharedsetting.AuthCaptchaTTLKey, "1.5", enum.CommonYes),
		}},
		{name: "non positive", rows: map[string]*systemsetting.Setting{
			sharedsetting.AuthCaptchaTTLKey: validCaptchaSetting(sharedsetting.AuthCaptchaTTLKey, "0", enum.CommonYes),
		}},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewSystemSettingCaptchaPolicyProvider(&fakeCaptchaPolicyRepository{rows: tt.rows})

			ttl, appErr := provider.TTL(context.Background())

			if appErr == nil {
				t.Fatalf("expected invalid setting to fail closed")
			}
			if ttl != 0 {
				t.Fatalf("expected zero ttl on failure, got %s", ttl)
			}
		})
	}
}

func TestSystemSettingCaptchaPolicyProviderWrapsRepositoryErrors(t *testing.T) {
	repoErr := errors.New("db down")
	provider := NewSystemSettingCaptchaPolicyProvider(&fakeCaptchaPolicyRepository{err: repoErr})

	_, appErr := provider.TTL(context.Background())

	if appErr == nil || appErr.Code != apperror.CodeInternal || !errors.Is(appErr, repoErr) {
		t.Fatalf("expected wrapped repository error, got %#v", appErr)
	}
	if appErr.MessageID != "captcha.policy.query_failed" {
		t.Fatalf("expected keyed query error, got %#v", appErr)
	}
}

func validCaptchaSetting(key string, value string, status int) *systemsetting.Setting {
	return &systemsetting.Setting{
		SettingKey:   key,
		SettingValue: value,
		ValueType:    enum.SystemSettingValueNumber,
		Status:       status,
		IsDel:        enum.CommonNo,
	}
}

type fakeCaptchaEngine struct {
	challenge *GeneratedCaptchaChallenge
	err       error
}

func (f fakeCaptchaEngine) Generate() (*GeneratedCaptchaChallenge, error) {
	return f.challenge, f.err
}

type fakeCaptchaStore struct {
	setID     string
	setSecret CaptchaChallengeSecret
	setTTL    time.Duration
	setTTLs   []time.Duration
	takeID    string
	secret    *CaptchaChallengeSecret
	secrets   []*CaptchaChallengeSecret
	setErr    error
	takeErr   error
}

func (f *fakeCaptchaStore) Set(ctx context.Context, id string, secret CaptchaChallengeSecret, ttl time.Duration) error {
	f.setID = id
	f.setSecret = secret
	f.setTTL = ttl
	f.setTTLs = append(f.setTTLs, ttl)
	return f.setErr
}

func (f *fakeCaptchaStore) Take(ctx context.Context, id string) (*CaptchaChallengeSecret, error) {
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
	store := &fakeCaptchaStore{}
	service := NewCaptchaService(
		fakeCaptchaEngine{challenge: &GeneratedCaptchaChallenge{
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
		WithCaptchaIDGenerator(func() (string, error) { return "captcha-id", nil }),
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
	store := &fakeCaptchaStore{}
	policy := &fakeCaptchaPolicy{ttlValues: []time.Duration{30 * time.Second, 75 * time.Second}}
	nextID := 0
	service := NewCaptchaService(
		fakeCaptchaEngine{challenge: &GeneratedCaptchaChallenge{Answer: Answer{X: 131, Y: 53}}},
		store,
		policy,
		WithCaptchaIDGenerator(func() (string, error) {
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
	store := &fakeCaptchaStore{secret: &CaptchaChallengeSecret{Answer: Answer{X: 120, Y: 80}}}
	service := NewCaptchaService(fakeCaptchaEngine{}, store, &fakeCaptchaPolicy{paddingValues: []int{3}})

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
	store := &fakeCaptchaStore{secrets: []*CaptchaChallengeSecret{
		{Answer: Answer{X: 120, Y: 80}},
		{Answer: Answer{X: 120, Y: 80}},
	}}
	policy := &fakeCaptchaPolicy{paddingValues: []int{3, 15}}
	service := NewCaptchaService(fakeCaptchaEngine{}, store, policy)

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
	store := &fakeCaptchaStore{}
	service := NewCaptchaService(fakeCaptchaEngine{}, store, &fakeCaptchaPolicy{})

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
	store := &fakeCaptchaStore{secret: &CaptchaChallengeSecret{Answer: Answer{X: 120, Y: 80}}}
	service := NewCaptchaService(fakeCaptchaEngine{}, store, &fakeCaptchaPolicy{paddingValues: []int{3}})

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 40, Y: 80},
	})

	if appErr == nil || appErr.Code != apperror.CodeBadRequest || appErr.Message != "验证码错误或已过期" {
		t.Fatalf("expected wrong captcha rejection, got %#v", appErr)
	}
}

func TestServiceVerifyErrorsCarryMessageIDs(t *testing.T) {
	service := NewCaptchaService(fakeCaptchaEngine{}, &fakeCaptchaStore{}, &fakeCaptchaPolicy{})

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
	service := NewCaptchaService(fakeCaptchaEngine{}, &fakeCaptchaStore{takeErr: errors.New("redis down")}, &fakeCaptchaPolicy{})

	appErr := service.Verify(context.Background(), VerifyInput{
		ID:     "captcha-id",
		Answer: &Answer{X: 120, Y: 80},
	})

	if appErr == nil || appErr.Code != apperror.CodeInternal || appErr.Message != "验证码校验失败" {
		t.Fatalf("expected internal captcha error, got %#v", appErr)
	}
}

func TestServiceReturnsServiceMissingWhenPolicyMissing(t *testing.T) {
	service := NewCaptchaService(fakeCaptchaEngine{}, &fakeCaptchaStore{}, nil)

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
