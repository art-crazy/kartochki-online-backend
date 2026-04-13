package routerai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"kartochki-online-backend/internal/config"
)

const (
	defaultEndpoint = "https://routerai.ru/api/v1"
	defaultTimeout  = 120 * time.Second
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
	// AspectRatio — соотношение сторон, например "3:4" для маркетплейс-карточек.
	// Если пусто, RouterAI использует "1:1" по умолчанию.
	AspectRatio string
	// ModelID — идентификатор модели в RouterAI, например "google/gemini-2.5-flash-image".
	ModelID string
}

// GenerateImage отправляет запрос к RouterAI и возвращает байты PNG-изображения.
// Ответ приходит в виде base64 data URL, клиент декодирует его в []byte.
func (c *Client) GenerateImage(ctx context.Context, input GenerateImageInput) ([]byte, error) {
	reqBody := chatCompletionRequest{
		Model: input.ModelID,
		Messages: []message{
			{
				Role:    "user",
				Content: input.Prompt,
			},
		},
		Modalities: []string{"image", "text"},
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create routerai request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("routerai request failed: %w", err)
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

	images := parsed.Choices[0].Message.Images
	if len(images) == 0 {
		return nil, fmt.Errorf("routerai returned no images in response (проверьте, поддерживает ли модель %q генерацию изображений)", input.ModelID)
	}

	dataURL := images[0].ImageURL.URL
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

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// --- типы запроса/ответа RouterAI (OpenAI-совместимый формат) ---

type chatCompletionRequest struct {
	Model       string      `json:"model"`
	Messages    []message   `json:"messages"`
	Modalities  []string    `json:"modalities"`
	ImageConfig *imageConfig `json:"image_config,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type imageConfig struct {
	AspectRatio string `json:"aspect_ratio,omitempty"`
}

type chatCompletionResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message assistantMessage `json:"message"`
}

type assistantMessage struct {
	Role    string      `json:"role"`
	Content string      `json:"content"`
	Images  []imageItem `json:"images"`
}

type imageItem struct {
	Type     string   `json:"type"`
	ImageURL imageURL `json:"image_url"`
}

type imageURL struct {
	URL string `json:"url"`
}
