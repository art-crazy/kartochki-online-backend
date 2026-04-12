package handlers

import (
	"net"
	"net/http"
	"strings"

	"kartochki-online-backend/internal/auth"
)

// sessionMetadataFromRequest собирает служебные данные сессии из HTTP-запроса.
func sessionMetadataFromRequest(r *http.Request) auth.SessionMetadata {
	return auth.SessionMetadata{
		UserAgent: strings.TrimSpace(r.UserAgent()),
		IPAddress: requestIPAddress(r),
	}
}

// requestIPAddress выбирает исходный IP, который можно показать в списке сессий.
func requestIPAddress(r *http.Request) string {
	forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}

	if host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return strings.TrimSpace(host)
	}

	return strings.TrimSpace(r.RemoteAddr)
}
