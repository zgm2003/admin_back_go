package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/shared/apperror"
)

// Session token hashing.
var ErrUnsafePepper = errors.New("token pepper is empty or unsafe")

func HashToken(token string, pepper string) (string, error) {
	if pepper == "" || pepper == "change_me_to_long_random" {
		return "", ErrUnsafePepper
	}

	sum := sha256.Sum256([]byte(token + "|" + pepper))
	return hex.EncodeToString(sum[:]), nil
}

func (a *SessionLifecycle) issueAccessToken(sessionID int64, userID int64, platform string, deviceID string, policy *AuthPolicy, now time.Time) (string, string, time.Time, *apperror.Error) {
	if a.accessCodec == nil {
		return "", "", time.Time{}, apperror.Unauthorized("Token认证未配置")
	}
	expiresAt := now.Add(policy.AccessTTL)
	accessToken, err := a.accessCodec.Issue(accesstoken.Claims{
		SessionID: sessionID,
		UserID:    userID,
		Platform:  platform,
		DeviceID:  deviceID,
		IssuedAt:  now,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", "", time.Time{}, apperror.Internal("访问令牌生成失败")
	}
	accessHash, err := HashToken(accessToken, a.tokenPepper)
	if err != nil {
		return "", "", time.Time{}, apperror.Unauthorized("令牌格式错误")
	}
	return accessToken, accessHash, expiresAt, nil
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

func matchClaims(session *Session, claims accesstoken.Claims) *apperror.Error {
	if session == nil {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if session.ID != claims.SessionID || session.UserID != claims.UserID {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if claims.Platform != "" && !strings.EqualFold(session.Platform, claims.Platform) {
		return apperror.Unauthorized("平台不匹配")
	}
	if claims.DeviceID != "" && session.DeviceID != "" && claims.DeviceID != session.DeviceID {
		return apperror.Unauthorized("设备变更，请重新登录")
	}
	return nil
}

func sessionIDPlaceholder(userID int64, platform string, now time.Time) string {
	return strconv.FormatInt(userID, 10) + "|" + platform + "|" + strconv.FormatInt(now.UnixNano(), 10)
}

func temporaryAccessTokenHash(seed string) string {
	sum := sha256.Sum256([]byte("pending|" + seed))
	return hex.EncodeToString(sum[:])
}

func makeToken(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
