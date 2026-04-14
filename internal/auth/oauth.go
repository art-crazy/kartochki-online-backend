package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const vkUserInfoURL = "https://api.vk.com/method/users.get"
const yandexUserInfoURL = "https://login.yandex.ru/info?format=json"
const oauthUserInfoTimeout = 10 * time.Second

// VKOAuthProfile содержит минимальные данные профиля VK ID.
type VKOAuthProfile struct {
	Subject   string
	Email     string
	Name      string
	AvatarURL string
}

// YandexOAuthProfile содержит минимальные данные профиля Яндекс ID.
type YandexOAuthProfile struct {
	Subject      string   `json:"id"`
	DefaultEmail string   `json:"default_email"`
	Emails       []string `json:"emails"`
	RealName     string   `json:"real_name"`
	DisplayName  string   `json:"display_name"`
	Login        string   `json:"login"`
	AvatarID     string   `json:"default_avatar_id"`
}

type vkUsersGetEnvelope struct {
	Response []struct {
		ID         int64  `json:"id"`
		FirstName  string `json:"first_name"`
		LastName   string `json:"last_name"`
		ScreenName string `json:"screen_name"`
		Photo200   string `json:"photo_200"`
	} `json:"response"`
	Error *vkAPIError `json:"error"`
}

type vkAPIError struct {
	Code    int    `json:"error_code"`
	Message string `json:"error_msg"`
}

func yandexAvatarURL(avatarID string) string {
	avatarID = strings.TrimSpace(avatarID)
	if avatarID == "" {
		return ""
	}

	return "https://avatars.yandex.net/get-yapic/" + avatarID + "/islands-200"
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

func oauthHTTPClient() *http.Client {
	return &http.Client{
		Timeout: oauthUserInfoTimeout,
	}
}
