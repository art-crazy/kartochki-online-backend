package routerai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"kartochki-online-backend/internal/config"
)

const (
	defaultEndpoint = "https://routerai.ru/api/v1"
	defaultTimeout  = 10 * time.Minute
	maxRespBodySize = 32 << 20 // 32 МБ — изображения могут быть большими
)

// Client выполняет запросы к RouterAI API для генерации изображений.
type Client struct {
	apiKey     string
	endpoint   string
	httpClient *http.Client
}

// New создаёт клиент RouterAI из конфигурации.
// Модель не хранится в клиенте — она передаётся per-request, чтобы пользователь мог выбирать модель при каждой генерации.
func New(cfg config.RouterAIConfig) *Client {
	endpoint := strings.TrimRight(cfg.Endpoint, "/")
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		apiKey:   cfg.APIKey,
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GenerateImageInput описывает параметры запроса на генерацию изображения.
type GenerateImageInput struct {
	// Prompt — текстовое описание желаемого изображения.
	Prompt string
	// SourceImageBody содержит байты исходника, который нужно учесть при генерации.
	SourceImageBody []byte
	// SourceImageMIMEType хранит MIME-тип исходника для data URL в image_url.
	SourceImageMIMEType string
	// AspectRatio — соотношение сторон, например "3:4" для маркетплейс-карточек.
	// Если пусто, RouterAI использует "1:1" по умолчанию.
	AspectRatio string
	// ModelID — идентификатор модели в RouterAI, например "google/gemini-2.5-flash-image".
	ModelID string
}

// GenerateImage отправляет запрос к RouterAI и возвращает байты PNG-изображения.
// Ответ приходит в виде base64 data URL, клиент декодирует его в []byte.
func (c *Client) GenerateImage(ctx context.Context, input GenerateImageInput) ([]byte, error) {
	content, err := buildMessageContent(input)
	if err != nil {
		return nil, err
	}

	reqBody := chatCompletionRequest{
		Model: input.ModelID,
		Messages: []message{
			{
				Role:    "user",
				Content: content,
			},
		},
		Modalities: requestModalities(input.ModelID),
	}

	// Соотношение сторон передаём только если оно задано явно.
	if input.AspectRatio != "" {
		reqBody.ImageConfig = &imageConfig{
			AspectRatio: input.AspectRatio,
		}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal routerai request: %w", err)
	}

	// Логируем ключевые поля запроса, чтобы в консоли backend было видно,
	// какой prompt и с какими параметрами ушёл в RouterAI.
	log.Printf(
		"routerai request: model=%q prompt=%q has_source_image=%t aspect_ratio=%q",
		input.ModelID,
		input.Prompt,
		len(input.SourceImageBody) > 0,
		input.AspectRatio,
	)

	resp, err := c.doChatCompletionRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxRespBodySize))
	if err != nil {
		return nil, fmt.Errorf("read routerai response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routerai returned status %d: %s", resp.StatusCode, truncate(string(respBody), 300))
	}

	var parsed chatCompletionResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("parse routerai response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("routerai returned no choices")
	}

	dataURL, err := extractImageDataURL(parsed.Choices[0].Message)
	if err != nil {
		return nil, fmt.Errorf("routerai returned no images in response (проверьте, поддерживает ли модель %q генерацию изображений): %w", input.ModelID, err)
	}
	imgBytes, err := decodeDataURL(dataURL)
	if err != nil {
		return nil, fmt.Errorf("decode routerai image: %w", err)
	}

	return imgBytes, nil
}

// decodeDataURL извлекает и декодирует base64-часть из строки вида "data:image/png;base64,<data>".
// Пробуем StdEncoding (с padding) и RawStdEncoding (без padding) — разные провайдеры могут опускать '='.
func decodeDataURL(dataURL string) ([]byte, error) {
	const prefix = "base64,"
	idx := strings.Index(dataURL, prefix)
	if idx < 0 {
		return nil, fmt.Errorf("unexpected image_url format (base64, prefix not found)")
	}
	raw := dataURL[idx+len(prefix):]
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("base64 decode: %w", err)
		}
	}
	return decoded, nil
}

// buildMessageContent собирает multimodal-content для RouterAI.
// Текст всегда идёт первым, чтобы модель получила задачу до ссылки на исходник.
func buildMessageContent(input GenerateImageInput) (any, error) {
	if len(input.SourceImageBody) == 0 {
		return input.Prompt, nil
	}

	dataURL, err := encodeImageDataURL(input.SourceImageBody, input.SourceImageMIMEType)
	if err != nil {
		return nil, fmt.Errorf("encode source image for routerai: %w", err)
	}

	content := []messagePart{
		{
			Type: "text",
			Text: input.Prompt,
		},
		{
			Type: "image_url",
			ImageURL: &imageURL{
				URL: dataURL,
			},
		},
	}

	return content, nil
}

// encodeImageDataURL подготавливает исходник для image_url в OpenAI-совместимом формате.
func encodeImageDataURL(body []byte, mimeType string) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("source image is empty")
	}

	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" {
		mimeType = "image/png"
	}

	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(body)), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// requestModalities возвращает модальности, совместимые с конкретной моделью RouterAI.
// Для image-only моделей запрашиваем только image: RouterAI сообщает, что комбинация
// image+text для них не маршрутизируется в доступный endpoint.
func requestModalities(modelID string) []string {
	switch modelID {
	case "black-forest-labs/flux.2-pro",
		"black-forest-labs/flux.2-klein-4b",
		"black-forest-labs/flux.2-max",
		"black-forest-labs/flux.2-flex",
		"bytedance-seed/seedream-4.5":
		return []string{"image"}
	}

	return []string{"image", "text"}
}
