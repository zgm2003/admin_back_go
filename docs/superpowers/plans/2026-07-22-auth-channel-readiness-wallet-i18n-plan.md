# Auth Channel Readiness, Captcha Recovery, and Wallet Locale Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hide unusable verification login methods, send real random phone codes through Tencent SMS, recover the captcha overlay according to stable error codes, and load Chinese wallet messages on `/profile/wallet`.

**Architecture:** Mail and SMS services own scene-aware readiness, while auth consumes a narrow readiness adapter plus separate mail and phone senders. Auth generates and caches one random code before delivery and rolls the cache back on sender failure. The frontend classifies only the two captcha machine codes as refreshable; all other failures reset the overlay, and wallet locale routing selects `payment` before the generic profile domain.

**Tech Stack:** Go 1.26.5, Gin, GORM, Tencent SES/SMS adapters, Redis code storage, Vue 3, TypeScript, Element Plus, vue-i18n, Vitest, Playwright, PowerShell 7, and the repository-owned `admin-dev` hot-reload supervisor

---

## File Structure

### Backend: `E:\admin\admin_back_go`

- Create `internal/module/mail/readiness.go`: mail scene readiness and pure config/template checks.
- Create `internal/module/mail/readiness_test.go`: absent, disabled, incomplete, ready, and repository-failure cases.
- Create `internal/module/sms/readiness.go`: SMS scene readiness and pure config/template checks.
- Create `internal/module/sms/readiness_test.go`: the equivalent SMS readiness matrix.
- Create `internal/module/sms/verification.go`: the real verification sender and shared Tencent delivery/log pipeline.
- Modify `internal/module/sms/service.go`: route `TestSend` through the shared delivery pipeline.
- Modify `internal/module/sms/service_test.go`: prove real verification parameters and log finalization.
- Create `internal/module/auth/verify_code_readiness.go`: map email/phone account types to channel readiness.
- Create `internal/module/auth/verify_code_readiness_test.go`: prove routing, propagation, and missing-provider failures.
- Modify `internal/module/auth/service.go`: filter login types, preflight send-code scenes, remove the fixed phone code, and call the phone sender.
- Modify `internal/module/auth/service_test.go`: lock ordering, pre-captcha rejection, generated-code delivery, and cache rollback.
- Modify `internal/platform/admin/build.go`: wire mail/SMS readiness and the real SMS sender into auth.
- Modify `internal/platform/admin/build_test.go`: guard the composition source.
- Modify `internal/shared/i18n/locales/zh-CN/auth.yaml`: Chinese unavailable-channel messages.
- Modify `internal/shared/i18n/locales/en-US/auth.yaml`: matching English messages.

### Frontend: `E:\admin\admin_front_ts`

- Create `src/modules/auth/captcha-error.ts`: stable-code captcha retry predicate.
- Create `tests/unit/auth/captcha-error.test.ts`: prove message text and generic errors cannot trigger a retry.
- Modify `src/components/SendCode/src/useCaptchaSendCode.ts`: shared overlay refresh/reset state machine.
- Modify `tests/shared/user/send-code-captcha-flow.test.ts`: behavioral coverage for shared send-code flows.
- Modify `src/views/Login/composables/useLoginForm.ts`: the same state machine for the login card.
- Modify `tests/shared/user/login-captcha-state.test.ts`: login-specific captcha behavior.
- Modify `src/i18n/index.ts`: map `/profile/wallet` to `payment` before `/profile` maps to `user`.
- Modify `tests/unit/i18n/lazy-locales.test.ts`: route mapping and actual Chinese wallet resolution.

No database migration, generated HTTP contract update, request/response shape change,
or locale-message relocation is required.

## Task 1: Add Mail Verification Readiness

**Files:**
- Create: `internal/module/mail/readiness.go`
- Create: `internal/module/mail/readiness_test.go`

- [ ] **Step 1: Write the failing mail readiness tests**

Reuse `fakeMailRepository`, `fakeMailSender`, and `testSecretBox` from
`service_test.go`. Add a complete fixture and table-driven unavailable cases:

```go
func readyMailConfig() *Config {
	return &Config{
		SecretIDEnc: "cipher-id", SecretKeyEnc: "cipher-key",
		Region: DefaultRegion, Endpoint: DefaultEndpoint,
		FromEmail: "noreply@example.com", VerifyCodeTTLMinutes: 5,
		Status: enum.CommonYes, IsDel: enum.CommonNo,
	}
}

func readyMailTemplate(scene string) *Template {
	return &Template{
		ID: 7, Scene: scene, Subject: "Verification code", TencentTemplateID: 12345,
		VariablesJSON: `["code","ttl_minutes"]`, Status: enum.CommonYes,
		IsDel: enum.CommonNo,
	}
}

func TestVerifyCodeReadyRequiresCompleteEnabledMailSetup(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Service, *fakeMailRepository)
	}{
		{name: "sender missing", mutate: func(s *Service, _ *fakeMailRepository) { s.sender = nil }},
		{name: "config missing", mutate: func(_ *Service, r *fakeMailRepository) { r.config = nil }},
		{name: "config disabled", mutate: func(_ *Service, r *fakeMailRepository) { r.config.Status = enum.CommonNo }},
		{name: "credential missing", mutate: func(_ *Service, r *fakeMailRepository) { r.config.SecretKeyEnc = "" }},
		{name: "sender address invalid", mutate: func(_ *Service, r *fakeMailRepository) { r.config.FromEmail = "bad" }},
		{name: "ttl invalid", mutate: func(_ *Service, r *fakeMailRepository) { r.config.VerifyCodeTTLMinutes = 0 }},
		{name: "template missing", mutate: func(_ *Service, r *fakeMailRepository) { delete(r.templates, enum.VerifyCodeSceneLogin) }},
		{name: "template disabled", mutate: func(_ *Service, r *fakeMailRepository) { r.templates[enum.VerifyCodeSceneLogin].Status = enum.CommonNo }},
		{name: "provider template missing", mutate: func(_ *Service, r *fakeMailRepository) { r.templates[enum.VerifyCodeSceneLogin].TencentTemplateID = 0 }},
		{name: "variables malformed", mutate: func(_ *Service, r *fakeMailRepository) { r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `["code"]` }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeMailRepository{
				config: readyMailConfig(),
				templates: map[string]*Template{enum.VerifyCodeSceneLogin: readyMailTemplate(enum.VerifyCodeSceneLogin)},
			}
			service := NewService(repo, testSecretBox(), &fakeMailSender{})
			tt.mutate(service, repo)
			ready, appErr := service.VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
			if appErr != nil || ready { t.Fatalf("ready=%v err=%#v", ready, appErr) }
		})
	}
}
```

Also add:

```go
func TestVerifyCodeReadyReturnsTrueForCompleteMailSetup(t *testing.T) {
	repo := &fakeMailRepository{
		config: readyMailConfig(),
		templates: map[string]*Template{
			enum.VerifyCodeSceneLogin: readyMailTemplate(enum.VerifyCodeSceneLogin),
		},
	}
	ready, appErr := NewService(repo, testSecretBox(), &fakeMailSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if appErr != nil || !ready {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyPropagatesMailRepositoryFailure(t *testing.T) {
	wantErr := errors.New("mail database unavailable")
	repo := &fakeMailRepository{config: readyMailConfig(), err: wantErr}
	ready, appErr := NewService(repo, testSecretBox(), &fakeMailSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if ready || appErr == nil || !errors.Is(appErr, wantErr) {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyRejectsNonMailScene(t *testing.T) {
	repo := &fakeMailRepository{config: readyMailConfig()}
	ready, appErr := NewService(repo, testSecretBox(), &fakeMailSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneBindPhone)
	if ready || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}
```

- [ ] **Step 2: Run the mail test to verify RED**

```powershell
go test ./internal/module/mail -run 'VerifyCodeReady' -count=1
```

Expected: compilation fails because `(*Service).VerifyCodeReady` does not exist.

- [ ] **Step 3: Implement mail readiness without decrypting secrets**

Implement this public capability and private pure checks in `readiness.go`:

```go
func (s *Service) VerifyCodeReady(ctx context.Context, scene string) (bool, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil { return false, appErr }
	if !enum.IsMailTemplateScene(scene) {
		return false, apperror.BadRequest("无效的邮件模板场景")
	}
	if s.sender == nil { return false, nil }
	cfg, err := repo.DefaultConfig(ctx)
	if err != nil {
		return false, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件配置失败", err)
	}
	if !mailConfigReady(cfg) { return false, nil }
	tmpl, err := repo.TemplateByScene(ctx, scene)
	if err != nil {
		return false, apperror.LegacyWrap(apperror.CodeInternal, http.StatusInternalServerError, "查询邮件模板失败", err)
	}
	return mailTemplateReady(tmpl), nil
}
```

`mailConfigReady` must require active status, non-empty encrypted credentials,
supported region, non-empty endpoint, valid sender/reply-to addresses, and TTL
within 1-60. `mailTemplateReady` must require active status, non-zero Tencent
template ID, non-empty subject, and decoded variables exactly
`code,ttl_minutes`. Malformed stored data returns `false`, not an operational
error. Credential decryption remains in the send path.

- [ ] **Step 4: Run the mail package to verify GREEN**

```powershell
go test ./internal/module/mail -count=1
```

Expected: all mail tests pass.

- [ ] **Step 5: Commit the mail readiness unit**

```powershell
git add internal/module/mail/readiness.go internal/module/mail/readiness_test.go
git commit -m "feat(mail): expose verification readiness"
```

## Task 2: Add SMS Verification Readiness

**Files:**
- Create: `internal/module/sms/readiness.go`
- Create: `internal/module/sms/readiness_test.go`
- Modify: `internal/module/sms/service_test.go`

- [ ] **Step 1: Extend the SMS fake repository for injected read failures**

Add `err error` to `fakeSmsRepository` and return it from `DefaultConfig` and
`TemplateByScene` before writing readiness tests:

```go
func (r *fakeSmsRepository) DefaultConfig(context.Context) (*Config, error) {
	return r.config, r.err
}

func (r *fakeSmsRepository) TemplateByScene(_ context.Context, scene string) (*Template, error) {
	return r.templates[scene], r.err
}
```

- [ ] **Step 2: Write the failing SMS readiness matrix**

Create complete fixtures equivalent to the mail fixtures:

```go
func readySMSConfig() *Config {
	return &Config{
		SecretIDEnc: "cipher-id", SecretKeyEnc: "cipher-key",
		SmsSdkAppID: "1400000000", SignName: "Admin",
		Region: DefaultRegion, Endpoint: DefaultEndpoint,
		VerifyCodeTTLMinutes: 5, Status: enum.CommonYes, IsDel: enum.CommonNo,
	}
}

func readySMSTemplate(scene string) *Template {
	return &Template{
		ID: 7, Scene: scene, TencentTemplateID: "12345",
		VariablesJSON: `["code","ttl_minutes"]`, Status: enum.CommonYes,
		IsDel: enum.CommonNo,
	}
}
```

Table cases must independently make sender, config, credential, SDK app ID,
sign, region, endpoint, TTL, template, Tencent template ID, status, and variables
unavailable. Use this concrete matrix:

```go
func TestVerifyCodeReadyRequiresCompleteEnabledSMSSetup(t *testing.T) {
	tests := []struct {
		name string
		mutate func(*Service, *fakeSmsRepository)
	}{
		{name: "sender missing", mutate: func(s *Service, _ *fakeSmsRepository) { s.sender = nil }},
		{name: "config missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config = nil }},
		{name: "config disabled", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.Status = enum.CommonNo }},
		{name: "credential missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.SecretIDEnc = "" }},
		{name: "app id missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.SmsSdkAppID = "" }},
		{name: "sign missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.SignName = "" }},
		{name: "region invalid", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.Region = "invalid" }},
		{name: "endpoint missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.Endpoint = "" }},
		{name: "ttl invalid", mutate: func(_ *Service, r *fakeSmsRepository) { r.config.VerifyCodeTTLMinutes = 61 }},
		{name: "template missing", mutate: func(_ *Service, r *fakeSmsRepository) { delete(r.templates, enum.VerifyCodeSceneLogin) }},
		{name: "template disabled", mutate: func(_ *Service, r *fakeSmsRepository) { r.templates[enum.VerifyCodeSceneLogin].Status = enum.CommonNo }},
		{name: "template id missing", mutate: func(_ *Service, r *fakeSmsRepository) { r.templates[enum.VerifyCodeSceneLogin].TencentTemplateID = "" }},
		{name: "variables malformed", mutate: func(_ *Service, r *fakeSmsRepository) { r.templates[enum.VerifyCodeSceneLogin].VariablesJSON = `{}` }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeSmsRepository()
			repo.config = readySMSConfig()
			repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)
			service := NewService(repo, secretbox.Box{}, &fakeSmsSender{})
			tt.mutate(service, repo)
			ready, appErr := service.VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
			if appErr != nil || ready { t.Fatalf("ready=%v err=%#v", ready, appErr) }
		})
	}
}

func TestVerifyCodeReadyReturnsTrueForCompleteSMSSetup(t *testing.T) {
	repo := newFakeSmsRepository()
	repo.config = readySMSConfig()
	repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)
	ready, appErr := NewService(repo, secretbox.Box{}, &fakeSmsSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if appErr != nil || !ready { t.Fatalf("ready=%v err=%#v", ready, appErr) }
}

func TestVerifyCodeReadyPropagatesSMSRepositoryFailure(t *testing.T) {
	wantErr := errors.New("sms database unavailable")
	repo := newFakeSmsRepository()
	repo.config, repo.err = readySMSConfig(), wantErr
	ready, appErr := NewService(repo, secretbox.Box{}, &fakeSmsSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneLogin)
	if ready || appErr == nil || !errors.Is(appErr, wantErr) {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}

func TestVerifyCodeReadyRejectsNonSMSScene(t *testing.T) {
	repo := newFakeSmsRepository()
	repo.config = readySMSConfig()
	ready, appErr := NewService(repo, secretbox.Box{}, &fakeSmsSender{}).
		VerifyCodeReady(context.Background(), enum.VerifyCodeSceneBindEmail)
	if ready || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("ready=%v err=%#v", ready, appErr)
	}
}
```

- [ ] **Step 3: Run the SMS test to verify RED**

```powershell
go test ./internal/module/sms -run 'VerifyCodeReady' -count=1
```

Expected: compilation fails because SMS does not expose `VerifyCodeReady`.

- [ ] **Step 4: Implement SMS readiness**

Use the same unavailable-versus-operational distinction as mail:

```go
func (s *Service) VerifyCodeReady(ctx context.Context, scene string) (bool, *apperror.Error) {
	repo, appErr := s.requireRepository()
	if appErr != nil { return false, appErr }
	if !enum.IsSmsTemplateScene(scene) {
		return false, badRequest("sms.scene.invalid", "无效的短信模板场景")
	}
	if s.sender == nil { return false, nil }
	cfg, err := repo.DefaultConfig(ctx)
	if err != nil { return false, wrapInternal("sms.config.query_failed", "查询短信配置失败", err) }
	if !smsConfigReady(cfg) { return false, nil }
	tmpl, err := repo.TemplateByScene(ctx, scene)
	if err != nil { return false, wrapInternal("sms.template.query_failed", "查询短信模板失败", err) }
	return smsTemplateReady(tmpl), nil
}
```

The pure helpers require the exact fields listed in the design and treat stored
validation drift as `false`. They do not call Tencent or decrypt credentials.

- [ ] **Step 5: Run the SMS package to verify GREEN**

```powershell
go test ./internal/module/sms -count=1
```

Expected: all SMS tests pass.

- [ ] **Step 6: Commit the SMS readiness unit**

```powershell
git add internal/module/sms/readiness.go internal/module/sms/readiness_test.go internal/module/sms/service_test.go
git commit -m "feat(sms): expose verification readiness"
```

## Task 3: Filter Auth Login Types And Preflight Send-Code Scenes

**Files:**
- Create: `internal/module/auth/verify_code_readiness.go`
- Create: `internal/module/auth/verify_code_readiness_test.go`
- Modify: `internal/module/auth/service.go`
- Modify: `internal/module/auth/service_test.go`
- Modify: `internal/shared/i18n/locales/zh-CN/auth.yaml`
- Modify: `internal/shared/i18n/locales/en-US/auth.yaml`

- [ ] **Step 1: Write failing tests for the readiness adapter**

Define the narrow contracts and expected routing in the test first:

```go
type fakeChannelReadinessProvider struct {
	ready  bool
	err    *apperror.Error
	scenes []string
}

func (f *fakeChannelReadinessProvider) VerifyCodeReady(_ context.Context, scene string) (bool, *apperror.Error) {
	f.scenes = append(f.scenes, scene)
	return f.ready, f.err
}
```

Tests must prove email calls only mail, phone calls only SMS, requested scenes
are forwarded unchanged, provider errors are returned unchanged, unknown account
types are rejected, and a missing selected provider is an internal error. Use:

```go
func TestChannelVerifyCodeReadinessProviderRoutesChannelAndScene(t *testing.T) {
	for _, tt := range []struct {
		name, accountType, scene string
		wantEmail, wantPhone bool
	}{
		{name: "email bind", accountType: LoginTypeEmail, scene: VerifyCodeSceneBindEmail, wantEmail: true},
		{name: "phone login", accountType: LoginTypePhone, scene: VerifyCodeSceneLogin, wantPhone: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			email := &fakeChannelReadinessProvider{ready: true}
			phone := &fakeChannelReadinessProvider{ready: true}
			provider := NewChannelVerifyCodeReadinessProvider(email, phone)
			ready, appErr := provider.VerifyCodeReady(context.Background(), tt.accountType, tt.scene)
			if appErr != nil || !ready { t.Fatalf("ready=%v err=%#v", ready, appErr) }
			if (len(email.scenes) == 1) != tt.wantEmail || (len(phone.scenes) == 1) != tt.wantPhone {
				t.Fatalf("email=%#v phone=%#v", email.scenes, phone.scenes)
			}
			selected := email
			if tt.wantPhone { selected = phone }
			if selected.scenes[0] != tt.scene { t.Fatalf("scenes=%#v", selected.scenes) }
		})
	}
}

func TestChannelVerifyCodeReadinessProviderPropagatesError(t *testing.T) {
	wantErr := apperror.Internal("query failed")
	provider := NewChannelVerifyCodeReadinessProvider(
		&fakeChannelReadinessProvider{err: wantErr},
		&fakeChannelReadinessProvider{ready: true},
	)
	ready, appErr := provider.VerifyCodeReady(context.Background(), LoginTypeEmail, VerifyCodeSceneLogin)
	if ready || appErr != wantErr { t.Fatalf("ready=%v err=%#v", ready, appErr) }
}

func TestChannelVerifyCodeReadinessProviderRejectsUnknownOrMissingProvider(t *testing.T) {
	provider := NewChannelVerifyCodeReadinessProvider(nil, &fakeChannelReadinessProvider{ready: true})
	if ready, appErr := provider.VerifyCodeReady(context.Background(), LoginTypeEmail, VerifyCodeSceneLogin); ready || appErr == nil || appErr.LegacyCode != apperror.CodeInternal {
		t.Fatalf("missing email ready=%v err=%#v", ready, appErr)
	}
	if ready, appErr := provider.VerifyCodeReady(context.Background(), LoginTypePassword, VerifyCodeSceneLogin); ready || appErr == nil || appErr.LegacyCode != apperror.CodeBadRequest {
		t.Fatalf("unknown type ready=%v err=%#v", ready, appErr)
	}
}
```

- [ ] **Step 2: Run the adapter tests to verify RED**

```powershell
go test ./internal/module/auth -run 'ChannelVerifyCodeReadinessProvider' -count=1
```

Expected: compilation fails because the readiness adapter types do not exist.

- [ ] **Step 3: Implement the narrow auth readiness adapter**

Create these contracts and implementation:

```go
type VerifyCodeReadinessProvider interface {
	VerifyCodeReady(ctx context.Context, accountType string, scene string) (bool, *apperror.Error)
}

type VerifyCodeChannelReadinessProvider interface {
	VerifyCodeReady(ctx context.Context, scene string) (bool, *apperror.Error)
}

type ChannelVerifyCodeReadinessProvider struct {
	email VerifyCodeChannelReadinessProvider
	phone VerifyCodeChannelReadinessProvider
}

func NewChannelVerifyCodeReadinessProvider(email, phone VerifyCodeChannelReadinessProvider) *ChannelVerifyCodeReadinessProvider {
	return &ChannelVerifyCodeReadinessProvider{email: email, phone: phone}
}
```

`VerifyCodeReady` switches on `LoginTypeEmail` and `LoginTypePhone`; it returns
keyed internal errors for a missing selected provider and a keyed bad request for
an unknown account type. Do not add sending or TTL methods to this interface.

- [ ] **Step 4: Write failing service tests for effective login types**

Add a service-level fake:

```go
type verifyCodeReadinessCall struct {
	accountType string
	scene       string
}

type fakeVerifyCodeReadinessProvider struct {
	readyByAccountType map[string]bool
	err                *apperror.Error
	calls              []verifyCodeReadinessCall
}

func (f *fakeVerifyCodeReadinessProvider) VerifyCodeReady(_ context.Context, accountType, scene string) (bool, *apperror.Error) {
	f.calls = append(f.calls, verifyCodeReadinessCall{accountType: accountType, scene: scene})
	if f.err != nil { return false, f.err }
	return f.readyByAccountType[accountType], nil
}
```

Add a table that configures platform types `email, phone, password`:

```go
func TestServiceLoginConfigFiltersUnavailableVerificationChannels(t *testing.T) {
	for _, tt := range []struct {
		name string
		ready map[string]bool
		want []string
	}{
		{name: "both unavailable", ready: map[string]bool{}, want: []string{LoginTypePassword}},
		{name: "email ready", ready: map[string]bool{LoginTypeEmail: true}, want: []string{LoginTypeEmail, LoginTypePassword}},
		{name: "phone ready", ready: map[string]bool{LoginTypePhone: true}, want: []string{LoginTypePhone, LoginTypePassword}},
		{name: "both ready", ready: map[string]bool{LoginTypeEmail: true, LoginTypePhone: true}, want: []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			readiness := &fakeVerifyCodeReadinessProvider{readyByAccountType: tt.ready}
			service := NewService(
				&fakeAuthRepository{},
				fakeLoginTypeProvider{types: []string{LoginTypePhone, LoginTypePassword, LoginTypeEmail}, captchaType: TypeSlide},
				&fakeSessionCreator{}, &fakeCaptchaVerifier{},
				WithVerifyCodeReadinessProvider(readiness),
			)
			result, appErr := service.LoginConfig(context.Background(), "admin")
			if appErr != nil { t.Fatalf("LoginConfig error=%#v", appErr) }
			if len(result.LoginTypeArr) != len(tt.want) { t.Fatalf("types=%#v", result.LoginTypeArr) }
			for i, want := range tt.want {
				if result.LoginTypeArr[i].Value != want { t.Fatalf("types=%#v", result.LoginTypeArr) }
			}
			for _, call := range readiness.calls {
				if call.scene != VerifyCodeSceneLogin { t.Fatalf("calls=%#v", readiness.calls) }
			}
		})
	}
}

func TestServiceLoginConfigPropagatesReadinessFailure(t *testing.T) {
	wantErr := apperror.Internal("mail repository unavailable")
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePassword}, captchaType: TypeSlide},
		&fakeSessionCreator{}, &fakeCaptchaVerifier{},
		WithVerifyCodeReadinessProvider(&fakeVerifyCodeReadinessProvider{err: wantErr}),
	)
	result, appErr := service.LoginConfig(context.Background(), "admin")
	if result != nil || appErr != wantErr { t.Fatalf("result=%#v err=%#v", result, appErr) }
}
```

Add login-time guards proving that only a selected code channel is queried and
that password login never depends on unrelated delivery configuration:

```go
func TestServiceCodeLoginRejectsUnavailableSelectedChannel(t *testing.T) {
	readiness := &fakeVerifyCodeReadinessProvider{readyByAccountType: map[string]bool{}}
	store := &fakeCodeStore{values: map[string]string{
		"auth:verify_code:phone:login:d521793014a021c7fec54bb8feee4885": "123456",
	}}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}, allowRegister: true},
		&fakeSessionCreator{}, &fakeCaptchaVerifier{},
		WithCodeStore(store), WithVerifyCodeReadinessProvider(readiness),
	)
	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271", LoginType: LoginTypePhone,
		Code: "123456", Platform: "admin",
	})
	if result != nil || appErr == nil || appErr.Code != "auth.verify_code.channel_unavailable" {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
	if len(readiness.calls) != 1 || readiness.calls[0] != (verifyCodeReadinessCall{accountType: LoginTypePhone, scene: VerifyCodeSceneLogin}) {
		t.Fatalf("calls=%#v", readiness.calls)
	}
	if store.deleted != "" { t.Fatalf("unavailable channel consumed code: %q", store.deleted) }
}

func TestServicePasswordLoginDoesNotQueryVerificationReadiness(t *testing.T) {
	readiness := &fakeVerifyCodeReadinessProvider{err: apperror.Internal("channel repository unavailable")}
	repo := &fakeAuthRepository{credential: &UserCredential{
		ID: 1, PasswordHash: phpBcryptHash(t, "123456"),
		Status: commonYes, IsDel: commonNo,
	}}
	service := NewService(
		repo,
		fakeLoginTypeProvider{types: []string{LoginTypeEmail, LoginTypePhone, LoginTypePassword}},
		&fakeSessionCreator{result: &TokenResult{AccessToken: "access-token"}},
		&fakeCaptchaVerifier{},
		WithVerifyCodeReadinessProvider(readiness),
	)
	result, appErr := service.Login(context.Background(), LoginInput{
		LoginAccount: "15671628271", LoginType: LoginTypePassword,
		Password: "123456", Platform: "admin",
	})
	if appErr != nil || result == nil || result.AccessToken != "access-token" {
		t.Fatalf("result=%#v err=%#v", result, appErr)
	}
	if len(readiness.calls) != 0 { t.Fatalf("calls=%#v", readiness.calls) }
}
```

Also assert readiness receives `VerifyCodeSceneLogin`, the existing closed
`email,phone,password` order is retained, and a readiness repository error makes
`LoginConfig` fail instead of returning partial data.

- [ ] **Step 5: Write failing tests for pre-captcha channel rejection**

For both a phone `login` request and an email `bind_email` request, return
`ready=false` and assert:

```go
func TestServiceSendCodeRejectsUnavailableChannelBeforeCaptcha(t *testing.T) {
	for _, tt := range []struct {
		name, account, accountType, scene, loginType string
	}{
		{name: "phone login", account: "15671628271", accountType: LoginTypePhone, scene: VerifyCodeSceneLogin, loginType: LoginTypePhone},
		{name: "email bind", account: "user@example.com", accountType: LoginTypeEmail, scene: VerifyCodeSceneBindEmail},
	} {
		t.Run(tt.name, func(t *testing.T) {
			verifier := &fakeCaptchaVerifier{}
			store := &fakeCodeStore{}
			readiness := &fakeVerifyCodeReadinessProvider{readyByAccountType: map[string]bool{}}
			service := NewService(
				&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{tt.loginType}},
				&fakeSessionCreator{}, verifier,
				WithCodeStore(store), WithVerifyCodeReadinessProvider(readiness),
			)
			message, appErr := service.SendCode(context.Background(), SendCodeInput{
				Account: tt.account, Scene: tt.scene, LoginType: tt.loginType,
				CaptchaID: "captcha-id", CaptchaAnswer: &Answer{X: 120, Y: 80},
			})
			if message != "" || appErr == nil || appErr.Code != "auth.verify_code.channel_unavailable" {
				t.Fatalf("message=%q err=%#v", message, appErr)
			}
			if verifier.calls != 0 || store.setKey != "" {
				t.Fatalf("verifier=%#v store=%#v", verifier, store)
			}
			if len(readiness.calls) != 1 || readiness.calls[0].accountType != tt.accountType || readiness.calls[0].scene != tt.scene {
				t.Fatalf("calls=%#v", readiness.calls)
			}
		})
	}
}
```

Add a scene-forwarding assertion showing `bind_email` is passed to mail rather
than rewritten to `login`. Update existing email/phone auth tests with a
ready-true fake option so they continue to describe explicitly ready channels.

- [ ] **Step 6: Run auth tests to verify RED**

```powershell
go test ./internal/module/auth -run 'LoginConfig|ChannelUnavailable|Readiness' -count=1
```

Expected: tests fail because auth does not filter or preflight channels.

- [ ] **Step 7: Implement effective login types and send preflight**

Add `verifyCodeReadiness` to `Service` plus this option:

```go
func WithVerifyCodeReadinessProvider(provider VerifyCodeReadinessProvider) Option {
	return func(s *Service) { s.verifyCodeReadiness = provider }
}
```

Implement `effectiveLoginTypes(ctx, configured, scene)` by iterating the existing
closed login order, retaining configured password, and querying readiness only
for configured email/phone. `LoginConfig` calls it with
`VerifyCodeSceneLogin`. `assertLoginTypeAllowed` first checks the selected type
against the platform policy; password returns without touching channel
readiness, while selected email/phone checks only that channel for the `login`
scene. `SendCode` validates account/scene/login-type matching, then checks
readiness for the actual request scene before checking captcha presence or
calling `CaptchaVerifier.Verify`.

Use one stable machine code and channel-specific localized messages:

```go
func unavailableVerifyCodeChannelError(accountType string) *apperror.Error {
	if accountType == LoginTypeEmail {
		return apperror.BadRequestKey("auth.verify_code.email_unavailable", nil, "邮箱验证码服务未配置").
			WithCode("auth.verify_code.channel_unavailable")
	}
	return apperror.BadRequestKey("auth.verify_code.phone_unavailable", nil, "短信验证码服务未配置").
		WithCode("auth.verify_code.channel_unavailable")
}
```

Add both message IDs in Chinese and English catalogs. Keep `CaptchaEnabled` and
captcha type behavior unchanged.

- [ ] **Step 8: Run auth and i18n tests to verify GREEN**

```powershell
go test ./internal/module/auth ./internal/shared/i18n -count=1
```

Expected: all selected packages pass, including catalog source coverage.

- [ ] **Step 9: Commit auth readiness filtering**

```powershell
git add internal/module/auth/verify_code_readiness.go internal/module/auth/verify_code_readiness_test.go internal/module/auth/service.go internal/module/auth/service_test.go internal/shared/i18n/locales/zh-CN/auth.yaml internal/shared/i18n/locales/en-US/auth.yaml
git commit -m "feat(auth): hide unavailable verification channels"
```

## Task 4: Send Real SMS Verification Codes

**Files:**
- Create: `internal/module/sms/verification.go`
- Modify: `internal/module/sms/service.go`
- Modify: `internal/module/sms/service_test.go`

- [ ] **Step 1: Write failing SMS verification delivery tests**

Use encrypted credentials, a `login` template, and `fakeSmsSender`. Add:

```go
func newVerificationSMSService(t *testing.T, sender *fakeSmsSender) (*Service, *fakeSmsRepository) {
	t.Helper()
	box := secretbox.New([]byte("12345678901234567890123456789012"))
	secretID, err := box.Encrypt("AKID")
	if err != nil { t.Fatal(err) }
	secretKey, err := box.Encrypt("SECRET")
	if err != nil { t.Fatal(err) }
	repo := newFakeSmsRepository()
	repo.config = readySMSConfig()
	repo.config.SecretIDEnc = secretID
	repo.config.SecretKeyEnc = secretKey
	repo.templates[enum.VerifyCodeSceneLogin] = readySMSTemplate(enum.VerifyCodeSceneLogin)
	return NewService(repo, box, sender), repo
}

func TestSendVerifyCodeUsesCodeTTLAndFinalizesSuccess(t *testing.T) {
	sender := &fakeSmsSender{result: SendResult{RequestID: "req-1", SerialNo: "serial-1", Fee: 1}}
	service, repo := newVerificationSMSService(t, sender)
	appErr := service.SendVerifyCode(
		context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 12*time.Minute,
	)
	if appErr != nil { t.Fatalf("SendVerifyCode error=%#v", appErr) }
	if !reflect.DeepEqual(sender.input.TemplateParamSet, []string{"654321", "12"}) {
		t.Fatalf("params=%#v", sender.input.TemplateParamSet)
	}
	if len(repo.createdLogs) != 1 {
		t.Fatalf("logs=%#v", repo.createdLogs)
	}
	created := repo.createdLogs[0]
	if created.Scene != enum.VerifyCodeSceneLogin || created.ToPhone != "+8613800138000" || created.Status != enum.SmsLogStatusPending {
		t.Fatalf("pending=%#v", created)
	}
	finish := repo.finishes[created.ID]
	if finish.Status != enum.SmsLogStatusSuccess || finish.RequestID != "req-1" || finish.SerialNo != "serial-1" || finish.Fee != 1 || finish.SentAt == nil {
		t.Fatalf("finish=%#v", finish)
	}
}

func TestSendVerifyCodeFinalizesProviderFailure(t *testing.T) {
	sender := &fakeSmsSender{
		result: SendResult{RequestID: "req-fail", SerialNo: "serial-fail", Fee: 1},
		err: fakeCodedError{code: "FailedOperation.TemplateIncorrect", message: "template incorrect"},
	}
	service, repo := newVerificationSMSService(t, sender)
	appErr := service.SendVerifyCode(
		context.Background(), enum.VerifyCodeSceneLogin, "13800138000", "654321", 5*time.Minute,
	)
	if appErr == nil || appErr.MessageID != "sms.send.failed" {
		t.Fatalf("error=%#v", appErr)
	}
	created := repo.createdLogs[0]
	finish := repo.finishes[created.ID]
	if finish.Status != enum.SmsLogStatusFailed || finish.RequestID != "req-fail" || finish.SerialNo != "serial-fail" || finish.ErrorCode != "FailedOperation.TemplateIncorrect" || finish.ErrorMessage != "template incorrect" {
		t.Fatalf("finish=%#v", finish)
	}
}

func TestSendVerifyCodeRejectsInvalidInputBeforeLogging(t *testing.T) {
	tests := []struct {
		name, scene, phone, code string
		ttl time.Duration
	}{
		{name: "scene", scene: enum.VerifyCodeSceneBindEmail, phone: "13800138000", code: "654321", ttl: 5*time.Minute},
		{name: "phone", scene: enum.VerifyCodeSceneLogin, phone: "bad", code: "654321", ttl: 5*time.Minute},
		{name: "empty code", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "", ttl: 5*time.Minute},
		{name: "non numeric code", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "abcdef", ttl: 5*time.Minute},
		{name: "short code", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "12345", ttl: 5*time.Minute},
		{name: "ttl low", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "654321", ttl: time.Second},
		{name: "ttl high", scene: enum.VerifyCodeSceneLogin, phone: "13800138000", code: "654321", ttl: 61*time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &fakeSmsSender{}
			service, repo := newVerificationSMSService(t, sender)
			appErr := service.SendVerifyCode(context.Background(), tt.scene, tt.phone, tt.code, tt.ttl)
			if appErr == nil || len(repo.createdLogs) != 0 || sender.input.PhoneNumber != "" {
				t.Fatalf("error=%#v logs=%#v sender=%#v", appErr, repo.createdLogs, sender.input)
			}
		})
	}
}
```

Add `bytes` and `encoding/json` to the test imports. At the end of the success
test, prove the code and decrypted credentials do not enter persisted or API log
representations:

```go
loggedValues := map[string]any{
	"created": repo.createdLogs,
	"stored":  repo.logs[created.ID],
	"finish":  finish,
	"dto":     logDTOFromRow(*repo.logs[created.ID]),
}
for name, value := range loggedValues {
	raw, err := json.Marshal(value)
	if err != nil { t.Fatalf("marshal %s: %v", name, err) }
	for _, sensitive := range [][]byte{[]byte("654321"), []byte("AKID"), []byte("SECRET")} {
		if bytes.Contains(raw, sensitive) {
			t.Fatalf("%s contains sensitive value: %s", name, raw)
		}
	}
}
```

- [ ] **Step 2: Run SMS verification tests to verify RED**

```powershell
go test ./internal/module/sms -run 'SendVerifyCode' -count=1
```

Expected: compilation fails because `SendVerifyCode` does not exist.

- [ ] **Step 3: Extract one shared delivery pipeline**

Create an internal input that distinguishes template scene, log scene, test
result recording, code, phone, and optional caller-supplied TTL:

```go
type verificationDeliveryInput struct {
	TemplateScene   string
	LogScene        string
	ToPhone         string
	Code            string
	TTL             time.Duration
	RecordTestResult bool
}
```

Implement:

```go
func (s *Service) SendVerifyCode(ctx context.Context, scene, toPhone, code string, ttl time.Duration) *apperror.Error {
	return s.sendVerificationCode(ctx, verificationDeliveryInput{
		TemplateScene: scene, LogScene: scene, ToPhone: toPhone, Code: code, TTL: ttl,
	})
}
```

`sendVerificationCode` validates all inputs, loads enabled config/template,
derives configured TTL only when `TTL == 0`, calls `templateParamsFromRow` with
`code` and `ttl_minutes`, decrypts credentials, creates the pending log, sends,
and finalizes success/failure. It updates `last_test_*` only when
`RecordTestResult` is true. It never stores or logs the code or parameter set.

- [ ] **Step 4: Route `TestSend` through the shared pipeline**

Replace the duplicated provider/log body with:

```go
func (s *Service) TestSend(ctx context.Context, input TestInput) *apperror.Error {
	return s.sendVerificationCode(ctx, verificationDeliveryInput{
		TemplateScene: strings.TrimSpace(input.TemplateScene),
		LogScene: enum.SmsSceneTest,
		ToPhone: input.ToPhone,
		Code: testCode,
		RecordTestResult: true,
	})
}
```

Preserve current `TestSend` success/failure metadata assertions and configured
TTL behavior. The only remaining fixed `123456` is `sms.testCode`, isolated to
this management test path.

- [ ] **Step 5: Run the complete SMS package to verify GREEN**

```powershell
go test ./internal/module/sms -count=1
```

Expected: existing test-send tests and new real-delivery tests all pass.

- [ ] **Step 6: Commit real SMS delivery**

```powershell
git add internal/module/sms/verification.go internal/module/sms/service.go internal/module/sms/service_test.go
git commit -m "feat(sms): send verification codes through Tencent"
```

## Task 5: Generate One Auth Code And Wire Phone Delivery

**Files:**
- Modify: `internal/module/auth/service.go`
- Modify: `internal/module/auth/service_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`

- [ ] **Step 1: Replace fixed-phone tests with failing real-sender tests**

Add a fake parallel to the mail sender:

```go
type fakeVerifyCodePhoneSender struct {
	scene string
	phone string
	code  string
	ttl   time.Duration
	err   *apperror.Error
}

func (f *fakeVerifyCodePhoneSender) SendVerifyCode(_ context.Context, scene, phone, code string, ttl time.Duration) *apperror.Error {
	f.scene, f.phone, f.code, f.ttl = scene, phone, code, ttl
	return f.err
}
```

Replace fixed-code expectations with:

```go
func TestServiceSendCodeGeneratesCachesAndSendsPhoneCode(t *testing.T) {
	store := &fakeCodeStore{}
	phoneSender := &fakeVerifyCodePhoneSender{}
	service := NewService(
		&fakeAuthRepository{},
		fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{},
		&fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodePhoneSender(phoneSender),
		WithVerifyCodeReadinessProvider(&fakeVerifyCodeReadinessProvider{
			readyByAccountType: map[string]bool{LoginTypePhone: true},
		}),
		WithVerifyCodePolicyProvider(&fakeVerifyCodePolicyProvider{
			ttlByAccountType: map[string]time.Duration{LoginTypePhone: 9 * time.Minute},
		}),
		WithVerifyCodeOptions(VerifyCodeOptions{
			CodeGenerator: func() (string, error) { return "654321", nil },
		}),
	)
	message, appErr := service.SendCode(
		context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone),
	)
	if appErr != nil || message != "验证码发送成功" {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if store.setCode != "654321" || store.setTTL != 9*time.Minute {
		t.Fatalf("store=%#v", store)
	}
	if phoneSender.scene != VerifyCodeSceneLogin || phoneSender.phone != "15671628271" || phoneSender.code != store.setCode || phoneSender.ttl != store.setTTL {
		t.Fatalf("sender=%#v store=%#v", phoneSender, store)
	}
}

func TestServiceSendCodeDeletesPhoneCodeWhenSMSFails(t *testing.T) {
	store := &fakeCodeStore{}
	wantErr := apperror.InternalKey("sms.send.failed", nil, "短信发送失败")
	service := NewService(
		&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{}, &fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodePhoneSender(&fakeVerifyCodePhoneSender{err: wantErr}),
		WithVerifyCodeReadinessProvider(&fakeVerifyCodeReadinessProvider{
			readyByAccountType: map[string]bool{LoginTypePhone: true},
		}),
		WithVerifyCodeOptions(VerifyCodeOptions{
			CodeGenerator: func() (string, error) { return "654321", nil },
		}),
	)
	message, appErr := service.SendCode(
		context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone),
	)
	if message != "" || appErr != wantErr {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if store.deleted != store.setKey || store.values[store.setKey] != "" {
		t.Fatalf("store=%#v", store)
	}
}

func TestServiceSendCodeGenerationFailureDoesNotCacheOrSend(t *testing.T) {
	store := &fakeCodeStore{}
	phoneSender := &fakeVerifyCodePhoneSender{}
	service := NewService(
		&fakeAuthRepository{}, fakeLoginTypeProvider{types: []string{LoginTypePhone}},
		&fakeSessionCreator{}, &fakeCaptchaVerifier{},
		WithCodeStore(store),
		WithVerifyCodePhoneSender(phoneSender),
		WithVerifyCodeReadinessProvider(&fakeVerifyCodeReadinessProvider{
			readyByAccountType: map[string]bool{LoginTypePhone: true},
		}),
		WithVerifyCodeOptions(VerifyCodeOptions{
			CodeGenerator: func() (string, error) { return "", errors.New("entropy unavailable") },
		}),
	)
	message, appErr := service.SendCode(
		context.Background(), validLoginSendCodeInput("15671628271", LoginTypePhone),
	)
	if message != "" || appErr == nil || appErr.Message != "验证码生成失败" {
		t.Fatalf("message=%q err=%#v", message, appErr)
	}
	if store.setKey != "" || phoneSender.phone != "" {
		t.Fatalf("store=%#v sender=%#v", store, phoneSender)
	}
}
```

Delete tests whose names or assertions require mock/fixed phone delivery.

- [ ] **Step 2: Run auth phone tests to verify RED**

```powershell
go test ./internal/module/auth -run 'Phone.*Code|SendCode.*Phone|GenerationFailure' -count=1
```

Expected: failures show phone still stores `123456` and never invokes an SMS
verification sender.

- [ ] **Step 3: Implement one generator and two delivery adapters**

Add the phone interface, field, and option:

```go
type VerifyCodePhoneSender interface {
	SendVerifyCode(ctx context.Context, scene, toPhone, code string, ttl time.Duration) *apperror.Error
}

func WithVerifyCodePhoneSender(sender VerifyCodePhoneSender) Option {
	return func(s *Service) { s.verifyCodePhoneSender = sender }
}
```

Remove `defaultPhoneCode` and `VerifyCodeOptions.PhoneCode`. Normalize only TTL
and generator. In `SendCode`, call `generateVerifyCode` for every account type,
cache once, select the matching sender, and delete the cache on missing sender or
sender error:

```go
code, err := s.generateVerifyCode()
if err != nil { return "", apperror.Internal("验证码生成失败") }
if err := s.codeStore.Set(ctx, cacheKey, code, ttl); err != nil {
	return "", apperror.LegacyWrap(
		apperror.CodeInternal, http.StatusInternalServerError, "验证码缓存写入失败", err,
	)
}

var sendErr *apperror.Error
if accountType == LoginTypeEmail {
	sendErr = s.verifyCodeMailSender.SendVerifyCode(ctx, input.Scene, input.Account, code, ttl)
} else {
	sendErr = s.verifyCodePhoneSender.SendVerifyCode(ctx, input.Scene, input.Account, code, ttl)
}
if sendErr != nil {
	_ = s.codeStore.Delete(ctx, cacheKey)
	return "", sendErr
}
```

Retain the injected generator for deterministic tests; production defaults to
the existing `crypto/rand` six-digit generator.

- [ ] **Step 4: Write and run the composition wiring test**

Add a source guard in `build_test.go` that requires all three options:

```go
for _, want := range []string{
	"auth.WithVerifyCodeMailSender(mailService)",
	"auth.WithVerifyCodePhoneSender(smsService)",
	"auth.WithVerifyCodeReadinessProvider(auth.NewChannelVerifyCodeReadinessProvider(mailService, smsService))",
} {
	if !strings.Contains(compact, want) { t.Fatalf("missing auth capability wiring %q", want) }
}
```

Then wire those exact options in `build.go`.

- [ ] **Step 5: Verify backend auth, SMS, and composition GREEN**

```powershell
go test ./internal/module/auth ./internal/module/mail ./internal/module/sms ./internal/platform/admin -count=1
$legacyAuthCodeRefs = & rg -n "defaultPhoneCode|PhoneCode" internal/module/auth 2>$null
$searchExit = $LASTEXITCODE
if ($searchExit -eq 0) {
    $legacyAuthCodeRefs
    throw 'legacy auth phone-code references remain'
}
if ($searchExit -ne 1) {
    throw "rg failed with exit code $searchExit"
}
```

Expected: all tests pass and the explicit no-match check exits cleanly.
`sms.testCode` may still exist only inside the SMS management test flow.

- [ ] **Step 6: Commit auth delivery and composition**

```powershell
git add internal/module/auth/service.go internal/module/auth/service_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go
git commit -m "feat(auth): deliver random phone verification codes"
```

## Task 6: Implement The Shared Captcha Error State Machine

**Files:**
- Create: `src/modules/auth/captcha-error.ts`
- Create: `tests/unit/auth/captcha-error.test.ts`
- Modify: `src/components/SendCode/src/useCaptchaSendCode.ts`
- Modify: `tests/shared/user/send-code-captcha-flow.test.ts`

- [ ] **Step 1: Write the failing stable-code predicate test**

```ts
import { createApiError } from '@/modules/http/error'
import { isCaptchaChallengeError } from '@/modules/auth/captcha-error'

const apiError = (code: string) => createApiError({
  kind: 'validation', code, retryable: false, messageKey: code, message: code,
})

expect(isCaptchaChallengeError(apiError('captcha.required'))).toBe(true)
expect(isCaptchaChallengeError(apiError('captcha.invalid_or_expired'))).toBe(true)
expect(isCaptchaChallengeError(apiError('auth.verify_code.channel_unavailable'))).toBe(false)
expect(isCaptchaChallengeError(new Error('验证码错误或已过期'))).toBe(false)
expect(isCaptchaChallengeError({ code: 'captcha.invalid_or_expired' })).toBe(false)
```

- [ ] **Step 2: Run the predicate test to verify RED**

```powershell
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' test -- tests/unit/auth/captcha-error.test.ts
```

Expected: module resolution fails because `captcha-error.ts` does not exist.

- [ ] **Step 3: Implement the predicate**

```ts
import { isApiError } from '@/modules/http/error'

const CAPTCHA_CHALLENGE_ERROR_CODES = new Set([
  'captcha.required',
  'captcha.invalid_or_expired',
])

export function isCaptchaChallengeError(error: unknown): boolean {
  return isApiError(error)
    && typeof error.code === 'string'
    && CAPTCHA_CHALLENGE_ERROR_CODES.has(error.code)
}
```

- [ ] **Step 4: Rewrite shared-flow tests around machine codes**

Change the current generic rejection test to use
`createApiError(...captcha.invalid_or_expired...)`. Add:

```ts
const replacementChallenge = {
  ...challenge,
  captcha_id: 'captcha-replacement',
  tile_x: 80,
}

const apiError = (code: string) => createApiError({
  kind: 'validation', code, retryable: false, messageKey: code, message: code,
})

it.each(['captcha.required', 'captcha.invalid_or_expired'])(
  'refreshes for %s',
  async (code) => {
    const onSent = vi.fn()
    const onError = vi.fn()
    const requestError = apiError(code)
    mocks.getCaptcha
      .mockResolvedValueOnce(challenge)
      .mockResolvedValueOnce(replacementChallenge)
    mocks.sendCode.mockRejectedValueOnce(requestError)
    const flow = useCaptchaSendCode({
      buildRequest: () => ({ account: '15671628271', scene: 'change_password' }),
      onSent,
      onError,
    })
    await flow.openCaptcha()
    flow.captchaX.value = 124
    await flow.completeCaptcha()
    expect(onSent).not.toHaveBeenCalled()
    expect(onError).toHaveBeenCalledWith(requestError, 'send')
    expect(mocks.getCaptcha).toHaveBeenCalledTimes(2)
    expect(flow.captchaDialogVisible.value).toBe(true)
    expect(flow.captchaChallenge.value).toEqual(replacementChallenge)
  },
)

it.each([
  'auth.verify_code.channel_unavailable',
  'http.network',
  'dependency.sms',
])('resets for %s', async (code) => {
  const onSent = vi.fn()
  const requestError = apiError(code)
  mocks.sendCode.mockRejectedValueOnce(requestError)
  const flow = useCaptchaSendCode({
    buildRequest: () => ({ account: '15671628271', scene: 'change_password' }),
    onSent,
  })
  await flow.openCaptcha()
  flow.captchaX.value = 124
  await flow.completeCaptcha()
  expect(onSent).not.toHaveBeenCalled()
  expect(mocks.getCaptcha).toHaveBeenCalledTimes(1)
  expect(flow.captchaDialogVisible.value).toBe(false)
  expect(flow.captchaChallenge.value).toBeNull()
  expect(flow.captchaX.value).toBe(0)
})

it('closes a blank overlay when captcha loading fails', async () => {
  const onError = vi.fn()
  const loadError = apiError('http.network')
  mocks.getCaptcha.mockRejectedValueOnce(loadError)
  const flow = useCaptchaSendCode({
    buildRequest: () => ({ account: 'user@example.com', scene: 'bind_email' }),
    onSent: vi.fn(),
    onError,
  })
  await flow.openCaptcha()
  expect(onError).toHaveBeenCalledWith(loadError, 'captcha')
  expect(flow.captchaDialogVisible.value).toBe(false)
  expect(flow.captchaChallenge.value).toBeNull()
  expect(flow.captchaX.value).toBe(0)
})
```

Every failure case asserts `onSent` is not called and no countdown-starting
success callback runs.

- [ ] **Step 5: Run shared-flow tests to verify RED**

```powershell
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' test -- tests/unit/auth/captcha-error.test.ts tests/shared/user/send-code-captcha-flow.test.ts
```

Expected: generic non-captcha errors still refresh and leave the overlay open;
captcha-load failure also leaves a blank overlay open.

- [ ] **Step 6: Implement shared refresh/reset behavior**

Import `isCaptchaChallengeError`. Make `refreshCaptcha` return success, call
`resetCaptcha` after a fetch failure, and preserve `pendingRequest` only while a
recognized send error refreshes successfully:

```ts
const refreshCaptcha = async (): Promise<boolean> => {
  clearChallenge()
  captchaLoading.value = true
  try {
    const challenge = await UsersApi.getCaptcha()
    captchaChallenge.value = challenge
    captchaX.value = challenge.tile_x
    return true
  } catch (error: unknown) {
    options.onError?.(error, 'captcha')
    resetCaptcha()
    return false
  } finally {
    captchaLoading.value = false
  }
}

// In completeCaptcha catch:
options.onError?.(error, 'send')
if (isCaptchaChallengeError(error)) await refreshCaptcha()
else resetCaptcha()
return
```

- [ ] **Step 7: Verify shared captcha GREEN**

Run the Task 6 focused command again. Expected: all predicate and flow tests
pass.

- [ ] **Step 8: Commit the shared frontend state machine**

```powershell
git add src/modules/auth/captcha-error.ts tests/unit/auth/captcha-error.test.ts src/components/SendCode/src/useCaptchaSendCode.ts tests/shared/user/send-code-captcha-flow.test.ts
git commit -m "fix(auth): reset captcha after non-captcha failures"
```

## Task 7: Apply The Captcha State Machine To Login

**Files:**
- Modify: `src/views/Login/composables/useLoginForm.ts`
- Modify: `tests/shared/user/login-captcha-state.test.ts`

- [ ] **Step 1: Write failing login captcha tests**

Import `createApiError`. Move the existing `firstChallenge` and
`replacementChallenge` fixtures out of the current single test into shared
test scope immediately after `mocks`, then remove their old local declarations:

```ts
const firstChallenge = {
  captcha_id: 'captcha-first',
  captcha_type: 'slide' as const,
  master_image: 'first-master',
  tile_image: 'first-tile',
  image_width: 320,
  image_height: 180,
  tile_x: 100,
  tile_y: 12,
  tile_width: 48,
  tile_height: 48,
  expires_in: 120,
}

const replacementChallenge = {
  ...firstChallenge,
  captcha_id: 'captcha-replacement',
  master_image: 'replacement-master',
  tile_image: 'replacement-tile',
  tile_x: 80,
}
```

Replace the message-only rejected challenge with a real API error and cover
both retry codes:

```ts
const apiError = (code: string, message: string) => createApiError({
  kind: 'validation', code, retryable: false, messageKey: code, message,
})

it.each(['captcha.required', 'captcha.invalid_or_expired'])(
  'keeps login captcha open and refreshes for %s',
  async (code) => {
    const { useLoginForm } = await import('@/views/Login/composables/useLoginForm')
    const login = useLoginForm()
    const completeSend = vi.fn()
    login.setFormRef({ validateField: vi.fn(async () => true), clearValidate: vi.fn() } as never)
    login.setSendCodeRef({ completeSend, reset: vi.fn() } as never)
    login.activeAccountType.value = 'phone'
    login.loginForm.login_account = '15671628271'
    login.captchaEnabled.value = true
    mocks.getCaptcha
      .mockResolvedValueOnce(firstChallenge)
      .mockResolvedValueOnce(replacementChallenge)
    mocks.sendCode.mockRejectedValueOnce(apiError(code, '验证码错误或已过期'))
    await login.requestLoginCode()
    login.captchaX.value = 124
    await login.completeCaptchaLogin()
    expect(completeSend).not.toHaveBeenCalled()
    expect(mocks.getCaptcha).toHaveBeenCalledTimes(2)
    expect(login.captchaDialogVisible.value).toBe(true)
    expect(login.captchaChallenge.value).toEqual(replacementChallenge)
  },
)
```

Add non-captcha reset and fetch-failure tests:

```ts
it('closes login captcha after an unavailable-channel error', async () => {
  const { useLoginForm } = await import('@/views/Login/composables/useLoginForm')
  const login = useLoginForm()
  login.setFormRef({ validateField: vi.fn(async () => true), clearValidate: vi.fn() } as never)
  login.setSendCodeRef({ completeSend: vi.fn(), reset: vi.fn() } as never)
  login.activeAccountType.value = 'phone'
  login.loginForm.login_account = '15671628271'
  login.captchaEnabled.value = true
  mocks.getCaptcha.mockResolvedValueOnce(firstChallenge)
  mocks.sendCode.mockRejectedValueOnce(apiError(
    'auth.verify_code.channel_unavailable',
    '短信验证码服务未配置',
  ))
  await login.requestLoginCode()
  login.captchaX.value = 124
  await login.completeCaptchaLogin()
  expect(mocks.messageError).toHaveBeenCalledWith('短信验证码服务未配置')
  expect(mocks.getCaptcha).toHaveBeenCalledTimes(1)
  expect(login.captchaDialogVisible.value).toBe(false)
  expect(login.captchaChallenge.value).toBeNull()
  expect(login.captchaX.value).toBe(0)
})

it('closes login captcha when the challenge cannot be loaded', async () => {
  const { useLoginForm } = await import('@/views/Login/composables/useLoginForm')
  const login = useLoginForm()
  login.setFormRef({ validateField: vi.fn(async () => true), clearValidate: vi.fn() } as never)
  login.activeAccountType.value = 'phone'
  login.loginForm.login_account = '15671628271'
  login.captchaEnabled.value = true
  mocks.getCaptcha.mockRejectedValueOnce(apiError('http.network', 'network failed'))
  await login.requestLoginCode()
  expect(mocks.messageError).toHaveBeenCalledWith('login.validation.captchaLoadFailed')
  expect(login.captchaDialogVisible.value).toBe(false)
  expect(login.captchaChallenge.value).toBeNull()
  expect(login.captchaX.value).toBe(0)
})
```

- [ ] **Step 2: Run login captcha tests to verify RED**

```powershell
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' test -- tests/shared/user/login-captcha-state.test.ts
```

Expected: unavailable-channel and captcha-fetch failures leave the overlay open.

- [ ] **Step 3: Implement login reset and stable-code retry**

Add one reset helper:

```ts
const resetCaptchaDialog = () => {
  captchaDialogVisible.value = false
  captchaChallenge.value = null
  captchaX.value = 0
}
```

Make `refreshCaptcha` catch fetch failures, show
`login.validation.captchaLoadFailed`, reset, and return `false`. On send failure,
show the API error once, then:

```ts
if (isCaptchaChallengeError(error)) {
  await refreshCaptcha()
} else {
  resetCaptchaDialog()
}
```

Use `resetCaptchaDialog` on success and tab change too, so all paths clear the
same state. Do not change password-login captcha behavior or login configuration
fetching.

- [ ] **Step 4: Verify login captcha GREEN**

```powershell
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' test -- tests/shared/user/login-captcha-state.test.ts tests/shared/user/send-code-captcha-flow.test.ts tests/component/login/LoginForm.test.ts
```

Expected: all selected login/send-code tests pass.

- [ ] **Step 5: Commit login captcha recovery**

```powershell
git add src/views/Login/composables/useLoginForm.ts tests/shared/user/login-captcha-state.test.ts
git commit -m "fix(login): close captcha on channel errors"
```

## Task 8: Load Payment Messages For The Profile Wallet

**Files:**
- Modify: `src/i18n/index.ts`
- Modify: `tests/unit/i18n/lazy-locales.test.ts`

- [ ] **Step 1: Change the existing expectation and add resolution tests**

Replace the incorrect assertion and add descendant/generic profile boundaries:

```ts
expect(localeModule.localeDomainForPath('/profile/wallet')).toBe('payment')
expect(localeModule.localeDomainForPath('/profile/wallet/ledger')).toBe('payment')
expect(localeModule.localeDomainForPath('/profile/security')).toBe('user')
```

Add an actual Chinese-message assertion:

```ts
await localeModule.ensureLocaleForRoute('zh-CN', '/profile/wallet')
expect(localeModule.default.global.t('wallet.summary')).toBe('钱包概览')
expect(localeModule.default.global.t('wallet.balance')).toBe('当前余额')
```

- [ ] **Step 2: Run locale tests to verify RED**

```powershell
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' test -- tests/unit/i18n/lazy-locales.test.ts
```

Expected: `/profile/wallet` still returns `user`, and `wallet.*` does not resolve.

- [ ] **Step 3: Add the narrow route exception**

Place this branch before generic profile matching:

```ts
if (hasPathPrefix(path, '/profile/wallet')) return 'payment'
```

Do not duplicate or move any wallet messages.

- [ ] **Step 4: Verify locale GREEN**

```powershell
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' test -- tests/unit/i18n/lazy-locales.test.ts tests/unit/i18n/locale-generator.test.ts
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' run locale:check
```

Expected: tests and locale parity/check commands pass.

- [ ] **Step 5: Commit wallet locale routing**

```powershell
git add src/i18n/index.ts tests/unit/i18n/lazy-locales.test.ts
git commit -m "fix(i18n): load payment messages for profile wallet"
```

## Task 9: Full Host Verification And `admin-dev` Acceptance

**Files:**
- No source files should change during this task.

- [ ] **Step 1: Format and inspect backend changes**

```powershell
gofmt -w internal/module/mail/readiness.go internal/module/mail/readiness_test.go internal/module/sms/readiness.go internal/module/sms/readiness_test.go internal/module/sms/verification.go internal/module/sms/service.go internal/module/sms/service_test.go internal/module/auth/verify_code_readiness.go internal/module/auth/verify_code_readiness_test.go internal/module/auth/service.go internal/module/auth/service_test.go internal/platform/admin/build.go internal/platform/admin/build_test.go
git diff --check
git status --short
```

Expected: no whitespace errors and only intentional backend files are modified.
If `gofmt` changes a committed file, commit the formatting in the owning task's
scope before continuing.

- [ ] **Step 2: Run complete backend host gates without Docker application builds**

```powershell
go test ./... -count=1
go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@v0.8.0-rc.1 ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
New-Item -ItemType Directory -Force .tmp\verify-bin | Out-Null
go build -trimpath -o .tmp/verify-bin/admin-api.exe ./cmd/admin-api
go build -trimpath -o .tmp/verify-bin/admin-worker.exe ./cmd/admin-worker
```

Expected: every command exits zero. Do not run `verify-backend.ps1`,
`verify-runtime-contracts.ps1`, `admin-up`, or another application-image build;
those paths invoke the slower Docker verification the user excluded for this
cycle.

- [ ] **Step 3: Run complete frontend host gates with pinned Node/npm**

```powershell
& 'E:\FlyEnv-Data\app\nodejs\v24.18.0\npm.cmd' run verify:frontend
git diff --check
git status --short
```

Expected: browser-only check, contract check, routes, locales, lint, quality,
typecheck, all tests with coverage, build, bundle check, test architecture, and
production dependency audit all pass.

- [ ] **Step 4: Start the hot-reload runtime**

In a dedicated PowerShell 7 terminal from `E:\admin`, run:

```powershell
admin-dev
```

If the profile shortcut is unavailable, run the repository entry directly:

```powershell
pwsh -NoProfile -File E:\admin\admin_back_go\scripts\admin-dev.ps1 -NoBrowser
```

Expected supervisor evidence:

```text
[WEB] Vite is ready at http://127.0.0.1:5173
[API] API health and readiness checks passed at http://127.0.0.1:8080/health and http://127.0.0.1:8080/ready
[WORKER] worker child process is stable
```

- [ ] **Step 5: Verify login configuration and wallet behavior in a real browser**

Use Playwright against `http://127.0.0.1:5173` and assert:

1. Login configuration returns password and omits each locally unconfigured
   email/SMS channel.
2. The login card renders only the returned methods; password login remains.
3. A mocked `captcha.invalid_or_expired` send response leaves the overlay open
   with a newly fetched challenge.
4. A mocked `auth.verify_code.channel_unavailable` send response shows the error
   and closes the overlay.
5. With an authenticated local Admin session, direct navigation and client
   navigation to `/profile/wallet` render `钱包概览` and `当前余额`, with no raw
   `wallet.*` keys and no console errors.
6. If the local SMS config and `login` template are complete, send one real code
   to the user-provided number and verify a finalized SMS log. If they are not
   configured, do not create credentials or templates; the hidden phone method
   is the required live result and automated sender tests prove delivery.

Do not record passwords, verification codes, Tencent credentials, or full
provider payloads in screenshots, terminal output, or committed evidence.

- [ ] **Step 6: Prove hot reload and stop cleanly**

Touch no source solely for this proof. While `admin-dev` is running, rely on the
actual implementation edits/restarts observed during Tasks 1-8, confirm `/ready`
returns success after the final Go rebuild, then press `Ctrl+C` in the supervisor
terminal.

Expected: ports 5173 and 8080 are released, API/worker/Vite stop, the
`.tmp/dev/admin-dev.lock.json` lock disappears, and MySQL/Redis state containers
remain running.

- [ ] **Step 7: Final repository integrity check**

```powershell
git -C E:\admin\admin_back_go status --short --branch
git -C E:\admin\admin_front_ts status --short --branch
git -C E:\admin\admin_back_go log -8 --oneline
git -C E:\admin\admin_front_ts log -5 --oneline
```

Expected: both worktrees are clean, each implementation commit is scoped, the
backend still contains the approved design/plan history, and nothing has been
pushed unless the user separately requests it.
