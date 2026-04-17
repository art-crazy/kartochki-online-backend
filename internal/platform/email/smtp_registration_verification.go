package email

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"text/template"
	"time"

	"kartochki-online-backend/internal/config"
)

const defaultRegistrationSupportEmail = "support@kartochki-online.ru"

// SendRegistrationVerificationEmail отправляет код подтверждения регистрации.
func (s *SMTPSender) SendRegistrationVerificationEmail(ctx context.Context, toEmail string, verificationID string, code string, expiresIn time.Duration) error {
	subject := "Код подтверждения для " + s.cfg.FromName
	plainText, htmlText, err := renderRegistrationVerificationEmail(registrationVerificationData{
		Code:          code,
		ServiceName:   s.cfg.FromName,
		ExpiresPhrase: buildExpirationPhrase(expiresIn),
		VerifyLink:    buildRegistrationVerifyLink(s.verifyURL, verificationID, code),
		SupportEmail:  registrationSupportEmail(s.cfg),
	})
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
	VerifyLink    string
	SupportEmail  string
}

var registrationVerificationPlainTmpl = template.Must(template.New("registration_verification_plain").Parse(
	`Ваш код подтверждения для {{ .ServiceName }}: {{ .Code }}.

Он действует {{ .ExpiresPhrase }}.

{{- if .VerifyLink }}
Если у вас уже открыта страница подтверждения, можно перейти по ссылке:
{{ .VerifyLink }}

{{- end }}
Если вы не регистрировались в {{ .ServiceName }}, просто проигнорируйте это письмо.

---
Вы получили это письмо, потому что зарегистрировались на сайте Kartochki-online.
Если у вас есть вопросы: {{ .SupportEmail }}
© Kartochki-online.ru
`))

var registrationVerificationHTMLTmpl = template.Must(template.New("registration_verification_html").Parse(
	`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Подтверждение регистрации</title></head>
<body style="font-family:sans-serif;color:#222;max-width:480px;margin:0 auto;padding:24px">
  <h2 style="margin-bottom:8px">Подтверждение регистрации</h2>
  <p>Ваш код для <strong>{{ .ServiceName }}</strong>:</p>
  <div style="font-size:32px;font-weight:700;letter-spacing:8px;line-height:1.2;margin:24px 0">{{ .Code }}</div>
  <p style="color:#666;font-size:14px">Код действует {{ .ExpiresPhrase }}.</p>
  {{- if .VerifyLink }}
  <p style="color:#666;font-size:14px">Если удобнее, откройте ссылку подтверждения:</p>
  <p style="margin:12px 0 24px">
    <a href="{{ .VerifyLink }}" style="color:#2563eb;word-break:break-all">{{ .VerifyLink }}</a>
  </p>
  {{- end }}
  <p style="color:#666;font-size:14px">Если вы не регистрировались в <strong>{{ .ServiceName }}</strong>, просто проигнорируйте это письмо.</p>
  <hr style="border:none;border-top:1px solid #eee;margin:24px 0">
  <p style="font-size:12px;color:#888;line-height:1.5">
    Вы получили это письмо, потому что зарегистрировались на сайте Kartochki-online.<br>
    Если у вас есть вопросы: {{ .SupportEmail }}<br>
    © Kartochki-online.ru
  </p>
</body>
</html>
`))

// renderRegistrationVerificationEmail рендерит письмо с кодом подтверждения регистрации.
func renderRegistrationVerificationEmail(data registrationVerificationData) (string, string, error) {
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

// buildExpirationPhrase переводит TTL в короткую фразу для шаблона письма.
func buildExpirationPhrase(expiresIn time.Duration) string {
	minutes := int(expiresIn / time.Minute)
	if minutes <= 0 {
		minutes = 1
	}

	return fmt.Sprintf("%d минут", minutes)
}

// buildRegistrationVerifyLink добавляет в fallback-ссылку и код, и verification_id.
// Так письмо не зависит от локального состояния браузера и может открыть экран подтверждения само по себе.
func buildRegistrationVerifyLink(baseURL string, verificationID string, code string) string {
	baseURL = strings.TrimSpace(baseURL)
	verificationID = strings.TrimSpace(verificationID)
	code = strings.TrimSpace(code)
	if baseURL == "" || verificationID == "" || code == "" {
		return ""
	}

	query := url.Values{
		"verification_id": []string{verificationID},
		"code":            []string{code},
	}

	return baseURL + "?" + query.Encode()
}

// registrationSupportEmail выбирает контакт для footer.
// Сначала используем Reply-To, а если он не задан, оставляем публичный адрес поддержки.
func registrationSupportEmail(cfg config.EmailConfig) string {
	replyTo := strings.TrimSpace(cfg.ReplyTo)
	if replyTo != "" {
		return replyTo
	}

	return defaultRegistrationSupportEmail
}
