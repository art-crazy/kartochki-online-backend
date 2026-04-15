package handlers

import "testing"

// TestIsValidPKCEVerifier проверяет базовые правила RFC 7636 для code_verifier.
func TestIsValidPKCEVerifier(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "valid minimum length", value: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOQ", want: true},
		{name: "valid allowed punctuation", value: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOQ-._~", want: true},
		{name: "too short", value: "short", want: false},
		{name: "too long", value: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz", want: false},
		{name: "invalid symbol", value: "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOQ!", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidPKCEVerifier(tt.value); got != tt.want {
				t.Fatalf("isValidPKCEVerifier() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsValidVKRedirectURI проверяет, что VK redirect_uri привязан к ожидаемому frontend origin.
func TestIsValidVKRedirectURI(t *testing.T) {
	tests := []struct {
		name           string
		value          string
		expectedOrigin string
		want           bool
	}{
		{name: "production origin", value: "https://kartochki-online.ru/auth", expectedOrigin: "https://kartochki-online.ru", want: true},
		{name: "wrong host", value: "https://evil.example/auth", expectedOrigin: "https://kartochki-online.ru", want: false},
		{name: "unexpected path", value: "https://kartochki-online.ru/auth/callback", expectedOrigin: "https://kartochki-online.ru", want: false},
		{name: "query is rejected", value: "https://kartochki-online.ru/auth?next=/app", expectedOrigin: "https://kartochki-online.ru", want: false},
		{name: "localhost for local frontend", value: "http://localhost:3000/auth", expectedOrigin: "http://localhost:3000", want: true},
		{name: "localhost rejected in production", value: "http://localhost:3000/auth", expectedOrigin: "https://kartochki-online.ru", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidVKRedirectURI(tt.value, tt.expectedOrigin); got != tt.want {
				t.Fatalf("isValidVKRedirectURI() = %v, want %v", got, tt.want)
			}
		})
	}
}
