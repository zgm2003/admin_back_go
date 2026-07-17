package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/shared/apperror"
)

const accessTokenIssuer = "admin_go"

// Session token hashing.
var ErrUnsafePepper = errors.New("token pepper is empty or unsafe")

func HashToken(token string, pepper string) (string, error) {
	if pepper == "" || pepper == "change_me_to_long_random" {
		return "", ErrUnsafePepper
	}

	sum := sha256.Sum256([]byte(token + "|" + pepper))
	return hex.EncodeToString(sum[:]), nil
}

func (a *SessionLifecycle) issueAccessToken(sessionID int64, userID int64, platform string, deviceID string, policy *AuthPolicy, now time.Time) (string, time.Time, *apperror.Error) {
	if a.accessCodec == nil {
		return "", time.Time{}, apperror.Unauthorized("Token认证未配置")
	}
	expiresAt := now.Add(policy.AccessTTL)
	accessToken, err := a.accessCodec.Issue(accesstoken.Claims{
		SessionID: sessionID,
		UserID:    userID,
		Issuer:    accessTokenIssuer,
		Platform:  platform,
		DeviceID:  deviceID,
		IssuedAt:  now,
		NotBefore: now,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", time.Time{}, apperror.Internal("访问令牌生成失败")
	}
	return accessToken, expiresAt, nil
}

func (a *SessionLifecycle) issueRefreshToken(policy *AuthPolicy, now time.Time) (string, string, time.Time, *apperror.Error) {
	refreshToken, err := a.tokenGenerator(64)
	if err != nil {
		return "", "", time.Time{}, apperror.Internal("刷新令牌生成失败")
	}
	refreshHash, err := HashToken(refreshToken, a.tokenPepper)
	if err != nil {
		return "", "", time.Time{}, apperror.Unauthorized("令牌格式错误")
	}
	return refreshToken, refreshHash, now.Add(policy.RefreshTTL), nil
}

func matchClaims(session *Session, claims accesstoken.Claims, now time.Time) *apperror.Error {
	if session == nil {
		return invalidAccessCredential()
	}
	if session.ID != claims.SessionID || session.UserID != claims.UserID || claims.Issuer != accessTokenIssuer {
		return invalidAccessCredential()
	}
	if !strings.EqualFold(session.Platform, claims.Platform) {
		return invalidAccessCredential()
	}
	if claims.DeviceID != session.DeviceID {
		return invalidAccessCredential()
	}
	if session.LastSeenAt.IsZero() || claims.IssuedAt.Unix() != session.LastSeenAt.Unix() || claims.NotBefore.Unix() != session.LastSeenAt.Unix() {
		return invalidAccessCredential()
	}
	if session.ExpiresAt.IsZero() || claims.ExpiresAt.Unix() != session.ExpiresAt.Unix() || !session.ExpiresAt.After(now) {
		return invalidAccessCredential()
	}
	if session.RevokedAt != nil || session.IsDel != commonNo || session.UserStatus != commonYes || session.UserIsDel != commonNo {
		return invalidAccessCredential()
	}
	if session.RefreshExpiresAt.IsZero() || !session.RefreshExpiresAt.After(now) {
		return invalidAccessCredential()
	}
	return nil
}

func invalidAccessCredential() *apperror.Error {
	return apperror.New(
		"auth.credential_mismatch",
		apperror.CategoryAuthentication,
		0,
		apperror.Permanent,
		"auth.token.invalid_or_expired",
		nil,
		"Token无效或已过期",
	)
}

func makeToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
