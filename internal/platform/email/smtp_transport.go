package email

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"
)

// dialSMTPWithTLS пытается подключиться к SMTP через все IP, которые вернул DNS.
//
// Это снижает вероятность случайного таймаута, когда один адрес провайдера временно
// недоступен, а остальные уже принимают соединения.
func (s *SMTPSender) dialSMTPWithTLS(deadline time.Time) (*tls.Conn, error) {
	lookupCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	ips, err := net.DefaultResolver.LookupIPAddr(lookupCtx, s.cfg.Host)
	if err != nil || len(ips) == 0 {
		// Если DNS-lookup не сработал, оставляем стандартный путь по имени хоста.
		return tls.DialWithDialer(
			&net.Dialer{Deadline: deadline},
			"tcp",
			s.smtpAddress(),
			s.smtpTLSConfig(),
		)
	}

	var joinedErr error
	for _, ip := range ips {
		if time.Now().After(deadline) {
			break
		}

		conn, dialErr := tls.DialWithDialer(
			&net.Dialer{Deadline: deadline},
			"tcp",
			net.JoinHostPort(ip.IP.String(), fmt.Sprintf("%d", s.cfg.Port)),
			s.smtpTLSConfig(),
		)
		if dialErr == nil {
			return conn, nil
		}

		// Сохраняем ошибки по всем IP, чтобы было видно, что проблема в сети,
		// а не в содержимом конкретного письма.
		joinedErr = errors.Join(joinedErr, fmt.Errorf("%s: %w", ip.IP.String(), dialErr))
	}

	if joinedErr == nil {
		joinedErr = fmt.Errorf("smtp deadline exceeded before connect")
	}

	return nil, joinedErr
}

// smtpAddress возвращает адрес SMTP-хоста в формате host:port.
func (s *SMTPSender) smtpAddress() string {
	return net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
}

// smtpTLSConfig создаёт TLS-конфигурацию с проверкой сертификата по имени хоста.
func (s *SMTPSender) smtpTLSConfig() *tls.Config {
	return &tls.Config{ServerName: s.cfg.Host}
}
