package admin

import authmodule "admin_back_go/internal/module/auth"

type LoginResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int64  `json:"expires_in"`
	RefreshToken     string `json:"refresh_token,omitempty"`
	RefreshExpiresIn int64  `json:"refresh_expires_in,omitempty"`
}

func presentLogin(result *authmodule.LoginResponse, variant authmodule.ClientVariant) *LoginResponse {
	if result == nil {
		return nil
	}
	response := &LoginResponse{AccessToken: result.AccessToken, ExpiresIn: int64(result.ExpiresIn)}
	if variant == authmodule.ClientDesktop {
		response.RefreshToken = result.RefreshToken
		response.RefreshExpiresIn = int64(result.RefreshExpiresIn)
	}
	return response
}

func presentCredentials(result *authmodule.TokenResult, variant authmodule.ClientVariant) *LoginResponse {
	if result == nil {
		return nil
	}
	response := &LoginResponse{AccessToken: result.AccessToken, ExpiresIn: int64(result.ExpiresIn)}
	if variant == authmodule.ClientDesktop {
		response.RefreshToken = result.RefreshToken
		response.RefreshExpiresIn = int64(result.RefreshExpiresIn)
	}
	return response
}
