package email

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"
)

// SendRegistrationVerificationEmail отправляет код подтверждения регистрации.
func (s *SMTPSender) SendRegistrationVerificationEmail(ctx context.Context, toEmail string, code string, expiresIn time.Duration) error {
	subject := "Код подтверждения для " + s.cfg.FromName
	plainText, htmlText, err := renderRegistrationVerificationEmail(code, expiresIn, s.cfg.FromName)
	if err != nil {
		return fmt.Errorf("render registration verification email: %w", err)
	}

	msg, err := buildMIMEMessage(s.cfg.FromAddress, s.cfg.FromName, toEmail, s.cfg.ReplyTo, subject, plainText, htmlText)
	if err != nil {
		return fmt.Errorf("build mime message: %w", err)
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}

	if err := s.sendWithTLS(toEmail, msg, deadline); err != nil {
		return fmt.Errorf("send registration verification email to %s: %w", toEmail, err)
	}

	return nil
}

// registrationVerificationData хранит данные для письма с кодом регистрации.
type registrationVerificationData struct {
	Code          string
	ServiceName   string
	ExpiresPhrase string
}

var registrationVerificationPlainTmpl = template.Must(template.New("registration_verification_plain").Parse(
	`Ваш код подтверждения для {{ .ServiceName }}: {{ .Code }}.

Он действует {{ .ExpiresPhrase }}.

Если вы не регистрировались в {{ .ServiceName }}, просто проигнорируйте это письмо.
`))

var registrationVerificationHTMLTmpl = template.Must(template.New("registration_verification_html").Parse(
	`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Подтверждение регистрации</title></head>
<body style="font-family:sans-serif;color:#222;max-width:480px;margin:0 auto;padding:24px">
  <h2 style="margin-bottom:8px">Подтверждение регистрации</h2>
  <p>Ваш код для <strong>{{ .ServiceName }}</strong>:</p>
  <p style="font-size:32px;font-weight:700;letter-spacing:6px;margin:24px 0">{{ .Code }}</p>
  <p style="color:#666;font-size:14px">Код действует {{ .ExpiresPhrase }}.</p>
  <p style="color:#666;font-size:14px">Если вы не регистрировались в <strong>{{ .ServiceName }}</strong>, просто проигнорируйте это письмо.</p>
</body>
</html>
`))

// renderRegistrationVerificationEmail рендерит письмо с кодом подтверждения регистрации.
func renderRegistrationVerificationEmail(code string, expiresIn time.Duration, serviceName string) (string, string, error) {
	minutes := int(expiresIn / time.Minute)
	if minutes <= 0 {
		minutes = 1
	}

	data := registrationVerificationData{
		Code:          code,
		ServiceName:   serviceName,
		ExpiresPhrase: fmt.Sprintf("%d минут", minutes),
	}

	var plain bytes.Buffer
	if err := registrationVerificationPlainTmpl.Execute(&plain, data); err != nil {
		return "", "", fmt.Errorf("render plain text: %w", err)
	}

	var html bytes.Buffer
	if err := registrationVerificationHTMLTmpl.Execute(&html, data); err != nil {
		return "", "", fmt.Errorf("render html: %w", err)
	}

	return plain.String(), html.String(), nil
}
