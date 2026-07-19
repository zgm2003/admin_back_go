package admin

import authmodule "admin_back_go/internal/module/auth"

type CredentialResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

func presentLogin(result *authmodule.LoginResponse) *CredentialResponse {
	if result == nil {
		return nil
	}
	return &CredentialResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   int64(result.ExpiresIn),
	}
}

func presentRefresh(result *authmodule.TokenResult) *CredentialResponse {
	if result == nil {
		return nil
	}
	return &CredentialResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   int64(result.ExpiresIn),
	}
}
