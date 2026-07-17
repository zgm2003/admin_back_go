package auth

import (
	"context"
	"strconv"
	"strings"
	"time"

	"admin_back_go/internal/config"
	"admin_back_go/internal/shared/apperror"
)

func (a *SessionLifecycle) Issue(ctx context.Context, input IssueCommand) (*CredentialSet, *apperror.Error) {
	if a == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	if input.UserID <= 0 {
		return nil, apperror.BadRequest("无效的用户ID")
	}
	input.Platform = strings.TrimSpace(input.Platform)
	if input.Platform == "" {
		return nil, apperror.BadRequest("无效的平台标识")
	}
	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	policy, policyErr := a.policyForSession(ctx, input.Platform)
	if policyErr != nil {
		return nil, policyErr
	}

	now := a.now()
	refreshToken, refreshHash, refreshExpiresAt, tokenErr := a.issueRefreshToken(policy, now)
	if tokenErr != nil {
		return nil, tokenErr
	}
	legacyNonce, nonceErr := makeToken(32)
	if nonceErr != nil {
		return nil, apperror.Internal("创建登录会话失败")
	}

	var (
		sessionID       int64
		accessToken     string
		accessExpiresAt time.Time
		revokedSessions []Session
		lifecycleErr    *apperror.Error
	)
	err := a.repository.WithUserLock(ctx, input.UserID, input.Platform, func(repository SessionRepository) error {
		activeSessions, listErr := repository.ListActiveForUpdate(ctx, input.UserID, input.Platform, now)
		if listErr != nil {
			return listErr
		}
		revokedSessions = sessionsToRevoke(activeSessions, policy)
		if revokeErr := repository.RevokeIDs(ctx, sessionIDs(revokedSessions), now); revokeErr != nil {
			return revokeErr
		}

		createdID, insertErr := repository.Insert(ctx, SessionCreate{
			UserID:           input.UserID,
			LegacyNonce:      legacyNonce,
			RefreshTokenHash: refreshHash,
			Platform:         input.Platform,
			DeviceID:         input.DeviceID,
			IP:               input.ClientIP,
			UserAgent:        input.UserAgent,
			LastSeenAt:       now,
			ExpiresAt:        now.Add(policy.AccessTTL),
			RefreshExpiresAt: refreshExpiresAt,
		})
		if insertErr != nil {
			return insertErr
		}
		sessionID = createdID
		issuedAccess, expiresAt, accessErr := a.issueAccessToken(sessionID, input.UserID, input.Platform, input.DeviceID, policy, now)
		if accessErr != nil {
			lifecycleErr = accessErr
			return accessErr
		}
		accessToken = issuedAccess
		accessExpiresAt = expiresAt
		return nil
	})
	if err != nil {
		if lifecycleErr != nil {
			return nil, lifecycleErr
		}
		return nil, apperror.Internal("创建登录会话失败")
	}

	a.deleteSessionCaches(ctx, revokedSessions)
	a.updateSingleSessionPointer(ctx, &Session{ID: sessionID, UserID: input.UserID, Platform: input.Platform})
	return &CredentialSet{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        int(accessExpiresAt.Sub(now).Seconds()),
		RefreshExpiresIn: int(policy.RefreshTTL.Seconds()),
	}, nil
}

func NewSessionLifecycle(deps LifecycleDeps) *SessionLifecycle {
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	deps.Config = config.NormalizeTokenConfig(deps.Config)
	tokenGenerator := deps.TokenGenerator
	if tokenGenerator == nil {
		tokenGenerator = makeToken
	}
	return &SessionLifecycle{
		cfg:            deps.Config,
		cache:          deps.Cache,
		repository:     deps.Repository,
		policyProvider: deps.PolicyProvider,
		accessCodec:    deps.AccessCodec,
		tokenPepper:    deps.TokenPepper,
		tokenGenerator: tokenGenerator,
		now:            now,
		loc:            time.Local,
	}
}

func (a *SessionLifecycle) Authenticate(ctx context.Context, input AccessCredential) (*Identity, *apperror.Error) {
	if a == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	if a.accessCodec == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	now := a.now()
	claims, err := a.accessCodec.Parse(input.AccessToken, now)
	if err != nil {
		return nil, apperror.Unauthorized("Token格式错误")
	}
	cacheKey := a.sessionCacheKey(claims.SessionID)
	if a.cache != nil {
		if session, cacheErr := a.sessionFromCache(ctx, cacheKey); cacheErr != nil {
			return nil, cacheErr
		} else if session != nil {
			if err := matchClaims(session, claims, now); err != nil {
				a.deleteCache(ctx, cacheKey)
				return nil, err
			}
			if policyErr := a.enforcePolicy(ctx, cacheKey, session, input); policyErr != nil {
				return nil, policyErr
			}
			return identityFromSession(session), nil
		}
	}

	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	session, repoErr := a.repository.FindValidByID(ctx, claims.SessionID, now)
	if repoErr != nil {
		return nil, apperror.Internal("Token会话查询失败")
	}
	if session == nil {
		return nil, apperror.Unauthorized("Token无效或已过期")
	}
	if !session.ExpiresAt.After(now) {
		return nil, apperror.Unauthorized("Token已过期")
	}
	if err := matchClaims(session, claims, now); err != nil {
		return nil, err
	}

	if policyErr := a.enforcePolicy(ctx, cacheKey, session, input); policyErr != nil {
		return nil, policyErr
	}

	if a.cache != nil {
		_ = a.cache.Set(ctx, cacheKey, cacheValue(session), a.cfg.SessionCacheTTL)
	}
	return identityFromSession(session), nil
}

func (a *SessionLifecycle) Rotate(ctx context.Context, input RotateCommand) (*CredentialSet, *apperror.Error) {
	if a == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	if strings.TrimSpace(input.RefreshToken) == "" {
		return nil, apperror.Unauthorized("缺少刷新令牌")
	}

	previousRefreshHash, err := HashToken(input.RefreshToken, a.tokenPepper)
	if err != nil {
		return nil, apperror.Unauthorized("令牌格式错误")
	}
	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}

	now := a.now()
	session, repoErr := a.repository.FindValidByRefreshHash(ctx, previousRefreshHash, now)
	if repoErr != nil {
		return nil, apperror.Internal("刷新令牌查询失败")
	}
	if session == nil {
		return nil, refreshReusedError()
	}
	if session.RevokedAt != nil || session.IsDel != commonNo || session.UserStatus != commonYes || session.UserIsDel != commonNo || !session.RefreshExpiresAt.After(now) {
		return nil, apperror.Unauthorized("刷新令牌已过期，请重新登录")
	}

	policy, policyErr := a.policyForSession(ctx, session.Platform)
	if policyErr != nil {
		return nil, policyErr
	}
	if policy.SingleSessionPerPlatform {
		if singleErr := a.enforceSingleSessionForRefresh(ctx, session); singleErr != nil {
			return nil, singleErr
		}
	}

	accessToken, accessExpiresAt, accessErr := a.issueAccessToken(session.ID, session.UserID, session.Platform, session.DeviceID, policy, now)
	if accessErr != nil {
		return nil, accessErr
	}
	refreshToken, nextRefreshHash, _, tokenErr := a.issueRefreshToken(policy, now)
	if tokenErr != nil {
		return nil, tokenErr
	}

	rotation := SessionRotation{
		RefreshTokenHash: nextRefreshHash,
		ExpiresAt:        accessExpiresAt,
		RefreshExpiresAt: session.RefreshExpiresAt,
		LastSeenAt:       now,
		IP:               input.ClientIP,
		UserAgent:        input.UserAgent,
	}
	won, err := a.repository.RotateIfRefreshHash(ctx, session.ID, previousRefreshHash, rotation)
	if err != nil {
		return nil, apperror.Internal("刷新令牌更新失败")
	}
	if !won {
		return nil, refreshReusedError()
	}

	a.deleteCache(ctx, a.sessionCacheKey(session.ID))
	a.updateSingleSessionPointer(ctx, session)

	return &CredentialSet{
		AccessToken:      accessToken,
		RefreshToken:     refreshToken,
		ExpiresIn:        int(policy.AccessTTL.Seconds()),
		RefreshExpiresIn: int(policy.RefreshTTL.Seconds()),
	}, nil
}

func refreshReusedError() *apperror.Error {
	return apperror.New(
		"auth.refresh_reused",
		apperror.CategoryAuthentication,
		0,
		apperror.Permanent,
		"auth.refresh_reused",
		nil,
		"刷新令牌已使用，请重新登录",
	)
}

func (a *SessionLifecycle) Revoke(ctx context.Context, command RevokeCommand) *apperror.Error {
	if a == nil || strings.TrimSpace(command.AccessToken) == "" {
		return nil
	}

	if a.accessCodec == nil {
		return nil
	}
	claims, err := a.accessCodec.Parse(command.AccessToken, a.now())
	if err != nil {
		return nil
	}
	if a.repository == nil {
		return nil
	}

	session, err := a.repository.FindValidByID(ctx, claims.SessionID, a.now())
	if err != nil || session == nil {
		return nil
	}
	if err := a.repository.Revoke(ctx, session.ID, a.now()); err != nil {
		return apperror.Internal("退出登录失败")
	}

	a.deleteCache(ctx, a.sessionCacheKey(session.ID))
	a.clearPointerIfMatches(ctx, session)
	return nil
}

func sessionsToRevoke(activeSessions []Session, policy *AuthPolicy) []Session {
	if policy == nil || len(activeSessions) == 0 {
		return nil
	}
	if policy.SingleSessionPerPlatform {
		return append([]Session(nil), activeSessions...)
	}
	if policy.MaxSessions <= 0 {
		return nil
	}
	overCount := len(activeSessions) - policy.MaxSessions + 1
	if overCount <= 0 {
		return nil
	}
	return append([]Session(nil), activeSessions[:overCount]...)
}

func sessionIDs(sessions []Session) []int64 {
	ids := make([]int64, 0, len(sessions))
	for _, session := range sessions {
		if session.ID > 0 {
			ids = append(ids, session.ID)
		}
	}
	return ids
}

func (a *SessionLifecycle) deleteSessionCaches(ctx context.Context, sessions []Session) {
	for _, session := range sessions {
		a.deleteCache(ctx, a.sessionCacheKey(session.ID))
	}
}

func (a *SessionLifecycle) sessionFromCache(ctx context.Context, cacheKey string) (*Session, *apperror.Error) {
	value, err := a.cache.Get(ctx, cacheKey)
	if err != nil {
		return nil, apperror.Internal("Token缓存读取失败")
	}
	if value == "" {
		return nil, nil
	}

	session, err := parseCachedSession(value, a.loc)
	if err != nil {
		_ = a.cache.Del(ctx, cacheKey)
		return nil, nil
	}
	if !session.ExpiresAt.After(a.now()) {
		_ = a.cache.Del(ctx, cacheKey)
		return nil, apperror.Unauthorized("Token已过期")
	}
	_ = a.cache.Expire(ctx, cacheKey, a.cfg.SessionCacheTTL)
	return session, nil
}

func (a *SessionLifecycle) enforcePolicy(ctx context.Context, cacheKey string, session *Session, input AccessCredential) *apperror.Error {
	if session == nil {
		return apperror.Unauthorized("Token无效或已过期")
	}
	if a.policyProvider == nil {
		return apperror.Unauthorized("平台策略未配置")
	}

	currentPlatform := strings.TrimSpace(input.Platform)
	if currentPlatform == "" {
		return apperror.BadRequest("无效的平台标识")
	}
	currentPolicy, err := a.policyProvider.Policy(ctx, currentPlatform)
	if err != nil {
		return apperror.Internal("平台策略查询失败")
	}
	if currentPolicy == nil {
		return apperror.BadRequest("无效的平台标识")
	}

	policy := currentPolicy
	if session.Platform != currentPlatform {
		policy, err = a.policyProvider.Policy(ctx, session.Platform)
		if err != nil {
			return apperror.Internal("平台策略查询失败")
		}
		if policy == nil {
			return apperror.Unauthorized("平台未配置或已禁用")
		}
	}

	if policy.BindPlatform && !strings.EqualFold(session.Platform, currentPlatform) {
		return apperror.Unauthorized("平台不匹配")
	}
	if policy.BindDevice && session.DeviceID != "" {
		if input.DeviceID == "" || input.DeviceID != session.DeviceID {
			return apperror.Unauthorized("设备变更，请重新登录")
		}
	}
	if policy.BindIP && session.IP != input.ClientIP {
		a.deleteCache(ctx, cacheKey)
		return apperror.Unauthorized("IP地址变动")
	}
	if policy.SingleSessionPerPlatform {
		if err := a.enforceSingleSession(ctx, cacheKey, session); err != nil {
			return err
		}
	}
	return nil
}

func (a *SessionLifecycle) policyForSession(ctx context.Context, platform string) (*AuthPolicy, *apperror.Error) {
	if a.policyProvider == nil {
		return nil, apperror.Unauthorized("平台策略未配置")
	}
	policy, err := a.policyProvider.Policy(ctx, platform)
	if err != nil {
		return nil, apperror.Internal("平台策略查询失败")
	}
	if policy == nil {
		return nil, apperror.Unauthorized("平台未配置或已禁用")
	}
	if policy.AccessTTL <= 0 {
		return nil, apperror.Internal("认证平台Token有效期未配置")
	}
	if policy.RefreshTTL <= 0 {
		return nil, apperror.Internal("认证平台Token有效期未配置")
	}
	return policy, nil
}

func (a *SessionLifecycle) enforceSingleSession(ctx context.Context, cacheKey string, session *Session) *apperror.Error {
	if a.cache == nil {
		return apperror.Unauthorized("Token认证未配置")
	}

	pointerKey := a.singleSessionPointerKey(session.Platform, session.UserID)
	allowedID, err := a.cache.Get(ctx, pointerKey)
	if err != nil {
		return apperror.Internal("单端登录指针读取失败")
	}

	if allowedID == "" {
		latest, latestErr := a.latestActiveSession(ctx, session.UserID, session.Platform)
		if latestErr != nil {
			return latestErr
		}
		if latest != nil {
			allowedID = strconv.FormatInt(latest.ID, 10)
			_ = a.cache.Set(ctx, pointerKey, allowedID, a.cfg.SingleSessionPointerTTL)
		}
	} else if !sameSessionID(allowedID, session.ID) {
		latest, latestErr := a.latestActiveSession(ctx, session.UserID, session.Platform)
		if latestErr != nil {
			return latestErr
		}
		if latest != nil && !sameSessionID(allowedID, latest.ID) {
			allowedID = strconv.FormatInt(latest.ID, 10)
			_ = a.cache.Set(ctx, pointerKey, allowedID, a.cfg.SingleSessionPointerTTL)
		} else if latest == nil {
			allowedID = ""
		}
	}

	if allowedID != "" && !sameSessionID(allowedID, session.ID) {
		a.deleteCache(ctx, cacheKey)
		return apperror.Unauthorized("账号已在其他设备登录")
	}
	return nil
}

func (a *SessionLifecycle) enforceSingleSessionForRefresh(ctx context.Context, session *Session) *apperror.Error {
	if a.cache == nil {
		return apperror.Unauthorized("Token认证未配置")
	}
	pointerKey := a.singleSessionPointerKey(session.Platform, session.UserID)
	allowedID, err := a.cache.Get(ctx, pointerKey)
	if err != nil {
		return apperror.Internal("单端登录指针读取失败")
	}
	if allowedID == "" {
		latest, latestErr := a.latestActiveSession(ctx, session.UserID, session.Platform)
		if latestErr != nil {
			return latestErr
		}
		if latest != nil {
			allowedID = strconv.FormatInt(latest.ID, 10)
			_ = a.cache.Set(ctx, pointerKey, allowedID, a.cfg.SingleSessionPointerTTL)
		}
	}
	if allowedID != "" && !sameSessionID(allowedID, session.ID) {
		return apperror.Unauthorized("账号已在其他设备登录，请重新登录")
	}
	return nil
}

func (a *SessionLifecycle) latestActiveSession(ctx context.Context, userID int64, platform string) (*Session, *apperror.Error) {
	if a.repository == nil {
		return nil, apperror.Unauthorized("Token认证未配置")
	}
	session, err := a.repository.FindLatestActiveByUserPlatform(ctx, userID, platform, a.now())
	if err != nil {
		return nil, apperror.Internal("单端登录会话查询失败")
	}
	return session, nil
}

func (a *SessionLifecycle) deleteCache(ctx context.Context, key string) {
	if a.cache != nil {
		_ = a.cache.Del(ctx, key)
	}
}

func (a *SessionLifecycle) updateSingleSessionPointer(ctx context.Context, session *Session) {
	if a.cache == nil || session == nil || session.ID <= 0 {
		return
	}
	key := a.singleSessionPointerKey(session.Platform, session.UserID)
	if cache, ok := a.cache.(monotonicSessionPointerCache); ok {
		_ = cache.SetIfGreater(ctx, key, session.ID, a.cfg.SingleSessionPointerTTL)
		return
	}
	_ = a.cache.Set(ctx, key, strconv.FormatInt(session.ID, 10), a.cfg.SingleSessionPointerTTL)
}

func (a *SessionLifecycle) clearPointerIfMatches(ctx context.Context, session *Session) {
	if a.cache == nil || session == nil {
		return
	}
	key := a.singleSessionPointerKey(session.Platform, session.UserID)
	current, err := a.cache.Get(ctx, key)
	if err == nil && sameSessionID(current, session.ID) {
		_ = a.cache.Del(ctx, key)
	}
}

func (a *SessionLifecycle) sessionCacheKey(sessionID int64) string {
	return a.cfg.RedisPrefix + "session:" + strconv.FormatInt(sessionID, 10)
}

func (a *SessionLifecycle) singleSessionPointerKey(platform string, userID int64) string {
	return a.cfg.RedisPrefix + "cur_sess:" + strings.ToLower(strings.TrimSpace(platform)) + ":" + strconv.FormatInt(userID, 10)
}

func sameSessionID(value string, sessionID int64) bool {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return err == nil && parsed == sessionID
}

func identityFromSession(session *Session) *Identity {
	return &Identity{
		UserID:    session.UserID,
		SessionID: session.ID,
		Platform:  session.Platform,
	}
}

// Deprecated compatibility entry points. New code depends on Lifecycle and its
// Issue/Rotate/Revoke verbs; these remain until all transports are cut over.
func NewAuthenticator(deps AuthenticatorDeps) *Authenticator {
	return NewSessionLifecycle(deps)
}

func (a *SessionLifecycle) Create(ctx context.Context, input CreateInput) (*TokenResult, *apperror.Error) {
	return a.Issue(ctx, input)
}

func (a *SessionLifecycle) Refresh(ctx context.Context, input RefreshInput) (*TokenResult, *apperror.Error) {
	return a.Rotate(ctx, input)
}

func (a *SessionLifecycle) Logout(ctx context.Context, accessToken string) *apperror.Error {
	return a.Revoke(ctx, RevokeCommand{AccessToken: accessToken})
}
