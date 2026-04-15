package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// vkTokenURL — единый endpoint VK ID для обмена authorization code на access token.
// Используется как widget flow, так и стандартным OAuth 2.0 PKCE flow.
const vkTokenURL = "https://id.vk.com/oauth2/auth"

type vkWidgetTokenResponse struct {
	AccessToken string `json:"access_token"`
	UserID      string `json:"-"`
	Email       string `json:"email"`
	Error       string `json:"error"`
	Description string `json:"error_description"`
}

func (r *vkWidgetTokenResponse) UnmarshalJSON(data []byte) error {
	var raw struct {
		AccessToken string          `json:"access_token"`
		UserID      json.RawMessage `json:"user_id"`
		Email       string          `json:"email"`
		Error       string          `json:"error"`
		Description string          `json:"error_description"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	r.AccessToken = raw.AccessToken
	r.Email = raw.Email
	r.Error = raw.Error
	r.Description = raw.Description
	userID, err := decodeVKUserID(raw.UserID)
	if err != nil {
		return err
	}
	r.UserID = userID
	return nil
}

func decodeVKUserID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), nil
	}

	var asNumber json.Number
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return "", fmt.Errorf("decode vk user_id: %w", err)
	}

	return asNumber.String(), nil
}

// exchangeVKToken отправляет form-параметры на vkTokenURL и возвращает распарсенный ответ.
// Конкретные поля формы (device_id, client_secret и т.д.) формирует вызывающая сторона.
func exchangeVKToken(ctx context.Context, form url.Values, callerName string) (vkWidgetTokenResponse, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, vkTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return vkWidgetTokenResponse{}, fmt.Errorf("create %s token request: %w", callerName, err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := oauthHTTPClient().Do(request)
	if err != nil {
		return vkWidgetTokenResponse{}, fmt.Errorf("request %s token: %w", callerName, err)
	}
	defer response.Body.Close()

	var tokenResponse vkWidgetTokenResponse
	if err := json.NewDecoder(response.Body).Decode(&tokenResponse); err != nil {
		if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnauthorized {
			return vkWidgetTokenResponse{}, ErrOAuthTokenInvalid
		}

		return vkWidgetTokenResponse{}, fmt.Errorf("decode %s token: %w", callerName, err)
	}

	if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnauthorized || tokenResponse.Error != "" {
		return vkWidgetTokenResponse{}, ErrOAuthTokenInvalid
	}
	if response.StatusCode != http.StatusOK {
		return vkWidgetTokenResponse{}, fmt.Errorf("%s token returned status %d", callerName, response.StatusCode)
	}
	if tokenResponse.AccessToken == "" || strings.TrimSpace(tokenResponse.UserID) == "" {
		return vkWidgetTokenResponse{}, fmt.Errorf("%s token returned incomplete identity", callerName)
	}

	return tokenResponse, nil
}

// fetchVKWidgetProfile проверяет короткий code от VK ID One Tap на стороне backend.
// VK ID связывает code с device_id, redirect_uri и PKCE verifier, поэтому все эти поля должны совпасть с frontend-настройкой SDK.
func fetchVKWidgetProfile(ctx context.Context, cfg OAuthProviderConfig, input VKWidgetLoginInput) (VKOAuthProfile, error) {
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
	form.Set("client_id", strings.TrimSpace(cfg.ClientID))
	form.Set("client_secret", strings.TrimSpace(cfg.ClientSecret))
	form.Set("device_id", deviceID)
	form.Set("code_verifier", codeVerifier)
	form.Set("redirect_uri", redirectURI)

	tokenResponse, err := exchangeVKToken(ctx, form, "vk widget")
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

func fetchVKProfileByAccessToken(ctx context.Context, accessToken string, userID string) (VKOAuthProfile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, vkUserInfoURL, nil)
	if err != nil {
		return VKOAuthProfile{}, fmt.Errorf("create vk userinfo request: %w", err)
	}

	query := request.URL.Query()
	query.Set("access_token", accessToken)
	query.Set("v", "5.199")
	query.Set("user_ids", userID)
	query.Set("fields", "screen_name,photo_200")
	request.URL.RawQuery = query.Encode()

	response, err := oauthHTTPClient().Do(request)
	if err != nil {
		return VKOAuthProfile{}, fmt.Errorf("request vk userinfo: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return VKOAuthProfile{}, ErrOAuthTokenInvalid
	}
	if response.StatusCode != http.StatusOK {
		return VKOAuthProfile{}, fmt.Errorf("vk userinfo returned status %d", response.StatusCode)
	}

	var envelope vkUsersGetEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return VKOAuthProfile{}, fmt.Errorf("decode vk userinfo: %w", err)
	}
	if envelope.Error != nil {
		// VK API может вернуть ошибку внутри JSON при HTTP 200. Ошибки авторизации
		// считаем отказом провайдера, остальные — неожиданным ответом интеграции.
		if envelope.Error.Code == 5 || envelope.Error.Code == 15 {
			return VKOAuthProfile{}, ErrOAuthTokenInvalid
		}

		return VKOAuthProfile{}, fmt.Errorf("vk userinfo returned api error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if len(envelope.Response) == 0 {
		return VKOAuthProfile{}, fmt.Errorf("vk userinfo returned empty response")
	}

	user := envelope.Response[0]
	subject := fmt.Sprintf("%d", user.ID)
	if subject != strings.TrimSpace(userID) {
		return VKOAuthProfile{}, fmt.Errorf("vk userinfo returned mismatched user id")
	}

	name := strings.TrimSpace(user.FirstName + " " + user.LastName)
	if name == "" {
		name = strings.TrimSpace(user.ScreenName)
	}

	return VKOAuthProfile{
		Subject:   subject,
		Name:      name,
		AvatarURL: user.Photo200,
	}, nil
}
