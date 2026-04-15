package auth

import (
	"context"
	"net/url"
	"strings"
)

// VKOAuthLoginInput содержит параметры стандартного VK OAuth 2.0 Authorization Code + PKCE flow.
// Device ID не нужен — это отличие от VK ID One Tap (widget flow).
type VKOAuthLoginInput struct {
	Code         string
	CodeVerifier string
	RedirectURI  string
}

// fetchVKOAuthProfile обменивает code на токен через VK ID OAuth 2.0 и загружает профиль пользователя.
// client_secret не передаётся: PKCE flow использует code_verifier вместо секрета клиента.
func fetchVKOAuthProfile(ctx context.Context, cfg OAuthProviderConfig, input VKOAuthLoginInput) (VKOAuthProfile, error) {
	code := strings.TrimSpace(input.Code)
	codeVerifier := strings.TrimSpace(input.CodeVerifier)
	redirectURI := strings.TrimSpace(input.RedirectURI)
	if code == "" || codeVerifier == "" || redirectURI == "" {
		return VKOAuthProfile{}, ErrOAuthTokenInvalid
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", strings.TrimSpace(cfg.ClientID))
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", redirectURI)

	tokenResponse, err := exchangeVKToken(ctx, form, "vk oauth")
	if err != nil {
		return VKOAuthProfile{}, err
	}

	profile, err := fetchVKProfileByAccessToken(ctx, tokenResponse.AccessToken, tokenResponse.UserID)
	if err != nil {
		return VKOAuthProfile{}, err
	}
	profile.Email = normalizeEmail(tokenResponse.Email)
	return profile, nil
}
