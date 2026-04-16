package auth

import (
	"context"
	"net/url"
	"strings"

	"github.com/rs/zerolog"
)

// VKOAuthLoginInput содержит параметры стандартного VK OAuth 2.0 Authorization Code + PKCE flow.
// Device ID возвращается VK в callback URL и должен передаваться при обмене кода на токен.
type VKOAuthLoginInput struct {
	Code         string
	DeviceID     string
	CodeVerifier string
	RedirectURI  string
}

// fetchVKOAuthProfile обменивает code на токен через VK ID OAuth 2.0 и загружает профиль пользователя.
// client_secret не передаётся: PKCE flow использует code_verifier вместо секрета клиента.
func fetchVKOAuthProfile(ctx context.Context, log zerolog.Logger, cfg OAuthProviderConfig, input VKOAuthLoginInput) (VKOAuthProfile, error) {
	code := strings.TrimSpace(input.Code)
	deviceID := strings.TrimSpace(input.DeviceID)
	codeVerifier := strings.TrimSpace(input.CodeVerifier)
	redirectURI := strings.TrimSpace(input.RedirectURI)
	if code == "" || deviceID == "" || codeVerifier == "" || redirectURI == "" {
		return VKOAuthProfile{}, ErrOAuthTokenInvalid
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("device_id", deviceID)
	form.Set("client_id", strings.TrimSpace(cfg.ClientID))
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", redirectURI)

	tokenResponse, err := exchangeVKToken(ctx, log, form, "vk oauth")
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
