package bootstrap

import (
	"context"

	"admin_back_go/internal/apperror"
	"admin_back_go/internal/config"
	"admin_back_go/internal/middleware"
	"admin_back_go/internal/module/authplatform"
	"admin_back_go/internal/module/session"
	"admin_back_go/internal/platform/database"
	"admin_back_go/internal/platform/redisclient"
)

func NewSessionAuthenticator(resources *Resources, cfg config.Config) *session.Authenticator {
	return session.NewAuthenticator(session.AuthenticatorDeps{
		Config:         cfg.Token,
		Cache:          session.NewRedisCache(resourcesTokenRedis(resources)),
		Repository:     session.NewGormRepository(resourcesDB(resources)),
		PolicyProvider: authplatform.NewService(authplatform.NewGormRepository(resourcesDB(resources))),
	})
}

func NewTokenAuthenticator(resources *Resources, cfg config.Config) middleware.TokenAuthenticator {
	return TokenAuthenticatorFor(NewSessionAuthenticator(resources, cfg))
}

func TokenAuthenticatorFor(authenticator *session.Authenticator) middleware.TokenAuthenticator {
	return func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
		identity, err := authenticator.Authenticate(ctx, session.TokenInput{
			AccessToken: input.AccessToken,
			Platform:    input.Platform,
			DeviceID:    input.DeviceID,
			ClientIP:    input.ClientIP,
		})
		if err != nil {
			return nil, err
		}
		if identity == nil {
			return nil, nil
		}
		return &middleware.AuthIdentity{
			UserID:    identity.UserID,
			SessionID: identity.SessionID,
			Platform:  identity.Platform,
		}, nil
	}
}

func resourcesRedis(resources *Resources) *redisclient.Client {
	if resources == nil {
		return nil
	}
	return resources.Redis
}

func resourcesTokenRedis(resources *Resources) *redisclient.Client {
	if resources == nil {
		return nil
	}
	return resources.TokenRedis
}

func resourcesDB(resources *Resources) *database.Client {
	if resources == nil {
		return nil
	}
	return resources.DB
}
