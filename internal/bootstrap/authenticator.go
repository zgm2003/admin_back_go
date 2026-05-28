package bootstrap

import (
	"context"

	"admin_back_go/internal/config"
	"admin_back_go/internal/infra/accesstoken"
	"admin_back_go/internal/infra/database"
	"admin_back_go/internal/infra/redisclient"
	"admin_back_go/internal/infra/secretkey"
	"admin_back_go/internal/middleware"
	authmodule "admin_back_go/internal/module/auth"
	"admin_back_go/internal/module/auth_platform"
	"admin_back_go/internal/shared/apperror"
)

func NewSessionAuthenticator(resources *Resources, cfg config.Config, keys *secretkey.KeyRing) *authmodule.Authenticator {
	var accessCodec accesstoken.Codec
	tokenPepper := ""
	if keys != nil {
		accessCodec = accesstoken.NewJWTCodec(keys.JWTSigningKey(), accesstoken.Options{Issuer: "admin_go"})
		tokenPepper = keys.TokenPepper()
	}
	return authmodule.NewAuthenticator(authmodule.AuthenticatorDeps{
		Config:         cfg.Token,
		Cache:          authmodule.NewSessionRedisCache(resourcesTokenRedis(resources)),
		Repository:     authmodule.NewSessionGormRepository(resourcesDB(resources)),
		PolicyProvider: authplatform.NewService(authplatform.NewGormRepository(resourcesDB(resources))),
		AccessCodec:    accessCodec,
		TokenPepper:    tokenPepper,
	})
}

func NewTokenAuthenticator(resources *Resources, cfg config.Config) middleware.TokenAuthenticator {
	keys, _ := secretkey.NewKeyRing(cfg.App.Secret)
	return TokenAuthenticatorFor(NewSessionAuthenticator(resources, cfg, keys))
}

func TokenAuthenticatorFor(authenticator *authmodule.Authenticator) middleware.TokenAuthenticator {
	return func(ctx context.Context, input middleware.TokenInput) (*middleware.AuthIdentity, *apperror.Error) {
		identity, err := authenticator.Authenticate(ctx, authmodule.TokenInput{
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
