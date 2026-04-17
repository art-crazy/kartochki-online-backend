package email

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"text/template"
	"time"

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/config"
)

// SMTPSender отправляет письма через SMTP с implicit TLS (порт 465).
//
// Соединение шифруется сразу при подключении — без STARTTLS.
// Это рекомендуемый режим для большинства современных провайдеров (Resend, Postmark, Mailgun).
type SMTPSender struct {
	cfg      config.EmailConfig
	resetURL string
	verifyURL string
}

// NewSMTPSender создаёт отправитель писем через SMTP.
//
// frontendURL — базовый адрес фронтенда, например "https://kartochki.online".
// Из него строится ссылка сброса пароля: frontendURL + "/reset-password?token=…".
func NewSMTPSender(cfg config.EmailConfig, frontendURL string) *SMTPSender {
	baseURL := strings.TrimRight(frontendURL, "/")
	resetURL := baseURL + "/reset-password"
	verifyURL := baseURL + "/verify"
	return &SMTPSender{cfg: cfg, resetURL: resetURL, verifyURL: verifyURL}
}

// SendPasswordResetEmail отправляет письмо со ссылкой для сброса пароля.
//
// Токен кодируется через url.QueryEscape, чтобы base64-символы (+, =) не ломали ссылку.
// Токен не логируется — безопасно для production.
func (s *SMTPSender) SendPasswordResetEmail(ctx context.Context, toEmail string, token string) error {
	resetLink := s.resetURL + "?token=" + url.QueryEscape(token)

	subject := "Сброс пароля на " + s.cfg.FromName
	plainText, htmlText, err := renderPasswordResetEmail(resetLink, s.cfg.FromName)
	if err != nil {
		return fmt.Errorf("render password reset email: %w", err)
	}

	msg, err := buildMIMEMessage(s.cfg.FromAddress, s.cfg.FromName, toEmail, s.cfg.ReplyTo, subject, plainText, htmlText)
	if err != nil {
		return fmt.Errorf("build mime message: %w", err)
	}

	// Используем deadline из контекста, если он установлен.
	// Это позволяет auth-сервису контролировать таймаут через context.WithTimeout.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(30 * time.Second)
	}

	if err := s.sendWithTLS(toEmail, msg, deadline); err != nil {
		return fmt.Errorf("send password reset email to %s: %w", toEmail, err)
	}

	return nil
}

// sendWithTLS устанавливает TLS-соединение и отправляет письмо через SMTP.
func (s *SMTPSender) sendWithTLS(toEmail string, msg []byte, deadline time.Time) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)

	conn, err := tls.DialWithDialer(
		&net.Dialer{Deadline: deadline},
		"tcp",
		addr,
		&tls.Config{ServerName: s.cfg.Host},
	)
	if err != nil {
		return fmt.Errorf("dial smtp tls %s: %w", addr, err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, s.cfg.Host)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if s.cfg.User != "" {
		smtpAuth := smtp.PlainAuth("", s.cfg.User, s.cfg.Password, s.cfg.Host)
		if err := client.Auth(smtpAuth); err != nil {
			return fmt.Errorf("smtp auth: %w", err)
		}
	}

	if err := client.Mail(s.cfg.FromAddress); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	if err := client.Rcpt(toEmail); err != nil {
		return fmt.Errorf("smtp RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		_ = w.Close()
		return fmt.Errorf("write email body: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close smtp data writer: %w", err)
	}

	return client.Quit()
}

// buildMIMEMessage формирует MIME-сообщение multipart/alternative.
//
// Обе части — plain-text и HTML — кодируются в quoted-printable,
// что гарантирует корректную передачу UTF-8 и длинных строк по RFC 2045.
// Граница (boundary) генерируется случайно, чтобы не совпасть с телом письма.
// replyTo — необязательный адрес для ответов; если пуст, заголовок не добавляется.
func buildMIMEMessage(fromAddr, fromName, toEmail, replyTo, subject, plainText, htmlText string) ([]byte, error) {
	boundary, err := randomBoundary()
	if err != nil {
		return nil, fmt.Errorf("generate mime boundary: %w", err)
	}

	encodedFrom := mime.QEncoding.Encode("utf-8", fromName)

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s <%s>\r\n", encodedFrom, fromAddr)
	fmt.Fprintf(&buf, "To: %s\r\n", toEmail)
	if replyTo != "" {
		fmt.Fprintf(&buf, "Reply-To: %s\r\n", replyTo)
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	buf.WriteString("MIME-Version: 1.0\r\n")
	fmt.Fprintf(&buf, "Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary)
	buf.WriteString("\r\n")

	// Plain-text часть — для почтовых клиентов без HTML.
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	if err := writeQP(&buf, plainText); err != nil {
		return nil, fmt.Errorf("encode plain text as quoted-printable: %w", err)
	}
	buf.WriteString("\r\n")

	// HTML часть — предпочтительная для современных клиентов.
	fmt.Fprintf(&buf, "--%s\r\n", boundary)
	buf.WriteString("Content-Type: text/html; charset=utf-8\r\n")
	buf.WriteString("Content-Transfer-Encoding: quoted-printable\r\n")
	buf.WriteString("\r\n")
	if err := writeQP(&buf, htmlText); err != nil {
		return nil, fmt.Errorf("encode html as quoted-printable: %w", err)
	}
	buf.WriteString("\r\n")

	fmt.Fprintf(&buf, "--%s--\r\n", boundary)

	return buf.Bytes(), nil
}

// writeQP кодирует строку в quoted-printable и пишет в buf.
func writeQP(buf *bytes.Buffer, text string) error {
	qpWriter := quotedprintable.NewWriter(buf)
	if _, err := qpWriter.Write([]byte(text)); err != nil {
		return err
	}
	return qpWriter.Close()
}

// randomBoundary генерирует случайную MIME-границу из 16 байт (32 hex-символа).
func randomBoundary() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// passwordResetData — данные для шаблонов письма сброса пароля.
type passwordResetData struct {
	ResetLink   string
	ServiceName string
}

var passwordResetPlainTmpl = template.Must(template.New("reset_plain").Parse(
	`Вы запросили сброс пароля на {{ .ServiceName }}.

Перейдите по ссылке, чтобы задать новый пароль:
{{ .ResetLink }}

Ссылка действительна 1 час. Если вы не запрашивали сброс — проигнорируйте это письмо.

---
Вы получили это письмо, потому что зарегистрированы на {{ .ServiceName }}.
`))

var passwordResetHTMLTmpl = template.Must(template.New("reset_html").Parse(
	`<!DOCTYPE html>
<html lang="ru">
<head><meta charset="utf-8"><title>Сброс пароля</title></head>
<body style="font-family:sans-serif;color:#222;max-width:480px;margin:0 auto;padding:24px">
  <h2 style="margin-bottom:8px">Сброс пароля</h2>
  <p>Вы запросили сброс пароля на <strong>{{ .ServiceName }}</strong>.</p>
  <p style="margin:24px 0">
    <a href="{{ .ResetLink }}"
       style="background:#2563eb;color:#fff;padding:12px 24px;border-radius:6px;text-decoration:none;font-weight:600">
      Задать новый пароль
    </a>
  </p>
  <p style="color:#666;font-size:13px">Ссылка действительна 1 час.<br>
  Если вы не запрашивали сброс — проигнорируйте это письмо.</p>
  <p style="color:#666;font-size:13px">Если кнопка не работает, скопируйте ссылку в браузер:<br>
  <a href="{{ .ResetLink }}" style="color:#2563eb;word-break:break-all">{{ .ResetLink }}</a></p>
  <hr style="border:none;border-top:1px solid #eee;margin:24px 0">
  <p style="color:#aaa;font-size:12px">Вы получили это письмо, потому что зарегистрированы на <strong>{{ .ServiceName }}</strong>.<br>
  Если это ошибка — просто проигнорируйте письмо.</p>
</body>
</html>
`))

// renderPasswordResetEmail рендерит plain-text и HTML версии письма сброса пароля.
func renderPasswordResetEmail(resetLink, serviceName string) (string, string, error) {
	data := passwordResetData{ResetLink: resetLink, ServiceName: serviceName}

	var plain bytes.Buffer
	if err := passwordResetPlainTmpl.Execute(&plain, data); err != nil {
		return "", "", fmt.Errorf("render plain text: %w", err)
	}

	var html bytes.Buffer
	if err := passwordResetHTMLTmpl.Execute(&html, data); err != nil {
		return "", "", fmt.Errorf("render html: %w", err)
	}

	return plain.String(), html.String(), nil
}

// Проверка на этапе компиляции: SMTPSender должен реализовывать auth.EmailSender.
var _ auth.EmailSender = (*SMTPSender)(nil)
