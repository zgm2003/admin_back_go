package auth

import (
	"context"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/shared/apperror"
)

// Lifecycle is the single public session boundary used by login, middleware,
// refresh/logout transport, and administrative composition.
type Lifecycle interface {
	Issue(context.Context, IssueCommand) (*CredentialSet, *apperror.Error)
	Authenticate(context.Context, AccessCredential) (*Identity, *apperror.Error)
	Rotate(context.Context, RotateCommand) (*CredentialSet, *apperror.Error)
	Revoke(context.Context, RevokeCommand) *apperror.Error
}

type AccessCredential struct {
	AccessToken string
	Platform    string
	DeviceID    string
	ClientIP    string
}

type Identity struct {
	UserID    int64
	SessionID int64
	Platform  string
}

type AuthPolicy struct {
	BindPlatform             bool
	BindDevice               bool
	BindIP                   bool
	SingleSessionPerPlatform bool
	MaxSessions              int
	AllowRegister            bool
	AccessTTL                time.Duration
	RefreshTTL               time.Duration
}

type PolicyProvider interface {
	Policy(ctx context.Context, platform string) (*AuthPolicy, error)
}

type LifecycleDeps struct {
	Config         config.TokenConfig
	Cache          SessionCache
	Repository     SessionRepository
	PolicyProvider PolicyProvider
	AccessCodec    accesstoken.Codec
	TokenPepper    string
	TokenGenerator TokenGenerator
	Now            func() time.Time
}

type SessionLifecycle struct {
	cfg            config.TokenConfig
	cache          SessionCache
	repository     SessionRepository
	policyProvider PolicyProvider
	accessCodec    accesstoken.Codec
	tokenPepper    string
	tokenGenerator TokenGenerator
	now            func() time.Time
	loc            *time.Location
}

type TokenGenerator func(bytes int) (string, error)

type RotateCommand struct {
	RefreshToken string
	ClientIP     string
	UserAgent    string
}

type IssueCommand struct {
	UserID    int64
	Platform  string
	DeviceID  string
	ClientIP  string
	UserAgent string
}

type RevokeCommand struct {
	AccessToken string
}

type CredentialSet struct {
	AccessToken      string `json:"access_token"`
	RefreshToken     string `json:"refresh_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
}

// Compatibility aliases keep existing service/transport callers source-stable
// while the canonical Lifecycle verbs replace the old authenticator naming.
type Authenticator = SessionLifecycle
type AuthenticatorDeps = LifecycleDeps
type TokenInput = AccessCredential
type RefreshInput = RotateCommand
type CreateInput = IssueCommand
type TokenResult = CredentialSet
