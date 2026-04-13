package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/oauth2"
)

const vkUserInfoURL = "https://api.vk.com/method/users.get"
const yandexUserInfoURL = "https://login.yandex.ru/info?format=json"
const oauthUserInfoTimeout = 10 * time.Second

// OAuthStateStore описывает минимальное хранилище одноразового state для OAuth.
type OAuthStateStore interface {
	SaveOAuthState(ctx context.Context, key string, ttl time.Duration) error
	ConsumeOAuthState(ctx context.Context, key string) (bool, error)
}

// VKOAuthProfile содержит минимальные данные профиля VK ID.
type VKOAuthProfile struct {
	Subject string
	Email   string
	Name    string
}

// YandexOAuthProfile содержит минимальные данные профиля Яндекс ID.
type YandexOAuthProfile struct {
	Subject      string   `json:"id"`
	DefaultEmail string   `json:"default_email"`
	Emails       []string `json:"emails"`
	RealName     string   `json:"real_name"`
	DisplayName  string   `json:"display_name"`
	Login        string   `json:"login"`
}

type vkUsersGetEnvelope struct {
	Response []struct {
		ID         int64  `json:"id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		ScreenName string `json:"screen_name"`
	} `json:"response"`
}

func newVKOAuthClient(cfg OAuthProviderConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://oauth.vk.com/authorize",
			TokenURL: "https://oauth.vk.com/access_token",
		},
		Scopes: []string{
			"email",
		},
	}
}

func fetchVKOAuthProfile(ctx context.Context, oauthConfig *oauth2.Config, code string) (VKOAuthProfile, error) {
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return VKOAuthProfile{}, fmt.Errorf("exchange vk oauth code: %w", err)
	}

	email, _ := token.Extra("email").(string)
	email = normalizeEmail(email)

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, vkUserInfoURL, nil)
	if err != nil {
		return VKOAuthProfile{}, fmt.Errorf("create vk userinfo request: %w", err)
	}

	query := request.URL.Query()
	query.Set("access_token", token.AccessToken)
	query.Set("v", "5.199")
	query.Set("fields", "screen_name")
	request.URL.RawQuery = query.Encode()

	response, err := oauthHTTPClient().Do(request)
	if err != nil {
		return VKOAuthProfile{}, fmt.Errorf("request vk userinfo: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return VKOAuthProfile{}, fmt.Errorf("vk userinfo returned status %d", response.StatusCode)
	}

	var envelope vkUsersGetEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return VKOAuthProfile{}, fmt.Errorf("decode vk userinfo: %w", err)
	}

	if len(envelope.Response) == 0 {
		return VKOAuthProfile{}, fmt.Errorf("vk userinfo returned empty response")
	}

	user := envelope.Response[0]
	name := strings.TrimSpace(strings.TrimSpace(user.FirstName + " " + user.LastName))
	if name == "" {
		name = strings.TrimSpace(user.ScreenName)
	}

	return VKOAuthProfile{
		Subject: fmt.Sprintf("%d", user.ID),
		Email:   email,
		Name:    name,
	}, nil
}

func newYandexOAuthClient(cfg OAuthProviderConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://oauth.yandex.ru/authorize",
			TokenURL: "https://oauth.yandex.ru/token",
		},
		// Яндекс email не гарантируется без явного запроса нужных прав.
		Scopes: []string{
			"login:email",
			"login:info",
		},
	}
}

func fetchYandexOAuthProfile(ctx context.Context, oauthConfig *oauth2.Config, code string) (YandexOAuthProfile, error) {
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return YandexOAuthProfile{}, fmt.Errorf("exchange yandex oauth code: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, yandexUserInfoURL, nil)
	if err != nil {
		return YandexOAuthProfile{}, fmt.Errorf("create yandex userinfo request: %w", err)
	}

	request.Header.Set("Authorization", "OAuth "+token.AccessToken)

	response, err := oauthHTTPClient().Do(request)
	if err != nil {
		return YandexOAuthProfile{}, fmt.Errorf("request yandex userinfo: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return YandexOAuthProfile{}, fmt.Errorf("yandex userinfo returned status %d", response.StatusCode)
	}

	var profile YandexOAuthProfile
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		return YandexOAuthProfile{}, fmt.Errorf("decode yandex userinfo: %w", err)
	}

	profile.DefaultEmail = normalizeEmail(profile.DefaultEmail)
	if profile.DefaultEmail == "" && len(profile.Emails) > 0 {
		profile.DefaultEmail = normalizeEmail(profile.Emails[0])
	}

	if strings.TrimSpace(profile.RealName) != "" {
		profile.RealName = strings.TrimSpace(profile.RealName)
	} else if strings.TrimSpace(profile.DisplayName) != "" {
		profile.RealName = strings.TrimSpace(profile.DisplayName)
	} else {
		profile.RealName = strings.TrimSpace(profile.Login)
	}

	return profile, nil
}

// fetchYandexOAuthProfileByToken загружает профиль Яндекс ID по access token, который выдал виджет.
func fetchYandexOAuthProfileByToken(ctx context.Context, accessToken string) (YandexOAuthProfile, error) {
	accessToken = strings.TrimSpace(accessToken)
	if accessToken == "" {
		return YandexOAuthProfile{}, ErrOAuthTokenInvalid
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, yandexUserInfoURL, nil)
	if err != nil {
		return YandexOAuthProfile{}, fmt.Errorf("create yandex userinfo request: %w", err)
	}

	request.Header.Set("Authorization", "OAuth "+accessToken)

	response, err := oauthHTTPClient().Do(request)
	if err != nil {
		return YandexOAuthProfile{}, fmt.Errorf("request yandex userinfo: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return YandexOAuthProfile{}, ErrOAuthTokenInvalid
	}

	if response.StatusCode != http.StatusOK {
		return YandexOAuthProfile{}, fmt.Errorf("yandex userinfo returned status %d", response.StatusCode)
	}

	var profile YandexOAuthProfile
	if err := json.NewDecoder(response.Body).Decode(&profile); err != nil {
		return YandexOAuthProfile{}, fmt.Errorf("decode yandex userinfo: %w", err)
	}

	profile.DefaultEmail = normalizeEmail(profile.DefaultEmail)
	if profile.DefaultEmail == "" && len(profile.Emails) > 0 {
		profile.DefaultEmail = normalizeEmail(profile.Emails[0])
	}

	if strings.TrimSpace(profile.RealName) != "" {
		profile.RealName = strings.TrimSpace(profile.RealName)
	} else if strings.TrimSpace(profile.DisplayName) != "" {
		profile.RealName = strings.TrimSpace(profile.DisplayName)
	} else {
		profile.RealName = strings.TrimSpace(profile.Login)
	}

	return profile, nil
}

func generateOAuthState() (string, error) {
	raw, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}

	return raw.String(), nil
}

func oauthStateKey(provider string, state string) string {
	return "oauth_state:" + provider + ":" + state
}

func isRedirectURLConfigured(value string) bool {
	redirectURL := strings.TrimSpace(value)
	if redirectURL == "" {
		return false
	}

	parsed, err := url.Parse(redirectURL)
	if err != nil {
		return false
	}

	return parsed.Scheme != "" && parsed.Host != ""
}

func oauthHTTPClient() *http.Client {
	return &http.Client{
		Timeout: oauthUserInfoTimeout,
	}
}
