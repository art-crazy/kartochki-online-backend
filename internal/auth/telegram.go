package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const telegramAuthFutureSkew = time.Minute

// TelegramLoginData содержит подписанные поля, которые frontend получает от Telegram Login Widget.
type TelegramLoginData struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
	PhotoURL  string
	AuthDate  int64
	Hash      string
}

// VerifyTelegramLogin проверяет подпись Telegram и свежесть данных.
//
// Telegram подписывает только фактически переданные поля. Поэтому в data-check-string
// мы включаем обязательные значения всегда, а необязательные строки только когда они не пустые.
func VerifyTelegramLogin(data TelegramLoginData, botToken string, maxAge time.Duration, now time.Time) error {
	if strings.TrimSpace(botToken) == "" {
		return ErrTelegramAuthNotConfigured
	}

	if data.ID <= 0 || data.AuthDate <= 0 || strings.TrimSpace(data.Hash) == "" {
		return ErrTelegramAuthInvalid
	}

	authTime := time.Unix(data.AuthDate, 0).UTC()
	if authTime.After(now.UTC().Add(telegramAuthFutureSkew)) {
		return ErrTelegramAuthInvalid
	}

	if now.UTC().After(authTime.Add(maxAge)) {
		return ErrTelegramAuthExpired
	}

	expectedHash, err := signTelegramLogin(data, botToken)
	if err != nil {
		return fmt.Errorf("sign telegram login: %w", err)
	}

	actualHash, err := hex.DecodeString(strings.TrimSpace(data.Hash))
	if err != nil {
		return ErrTelegramAuthInvalid
	}

	expectedHashBytes, err := hex.DecodeString(expectedHash)
	if err != nil {
		return fmt.Errorf("decode expected telegram hash: %w", err)
	}

	if !hmac.Equal(expectedHashBytes, actualHash) {
		return ErrTelegramAuthInvalid
	}

	return nil
}

// BuildTelegramDisplayName собирает понятное имя пользователя из данных Telegram.
func BuildTelegramDisplayName(data TelegramLoginData) string {
	fullName := strings.TrimSpace(strings.TrimSpace(data.FirstName) + " " + strings.TrimSpace(data.LastName))
	if fullName != "" {
		return fullName
	}

	if username := strings.TrimSpace(data.Username); username != "" {
		return username
	}

	return fmt.Sprintf("Telegram %d", data.ID)
}

func signTelegramLogin(data TelegramLoginData, botToken string) (string, error) {
	dataCheckString := buildTelegramDataCheckString(data)

	secretKey := sha256.Sum256([]byte(botToken))
	mac := hmac.New(sha256.New, secretKey[:])
	if _, err := mac.Write([]byte(dataCheckString)); err != nil {
		return "", err
	}

	return hex.EncodeToString(mac.Sum(nil)), nil
}

func buildTelegramDataCheckString(data TelegramLoginData) string {
	fields := map[string]string{
		"auth_date": strconv.FormatInt(data.AuthDate, 10),
		"id":        strconv.FormatInt(data.ID, 10),
	}

	if value := strings.TrimSpace(data.FirstName); value != "" {
		fields["first_name"] = value
	}
	if value := strings.TrimSpace(data.LastName); value != "" {
		fields["last_name"] = value
	}
	if value := strings.TrimSpace(data.Username); value != "" {
		fields["username"] = value
	}
	if value := strings.TrimSpace(data.PhotoURL); value != "" {
		fields["photo_url"] = value
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+fields[key])
	}

	return strings.Join(lines, "\n")
}
