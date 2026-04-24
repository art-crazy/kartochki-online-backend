package routerai

import (
	"encoding/json"
	"fmt"
	"strings"
)

// extractImageDataURL достаёт data URL картинки из разных форматов ответа RouterAI.
// Часть моделей кладёт результат в message.images, а часть — прямо в message.content.
func extractImageDataURL(msg assistantMessage) (string, error) {
	if len(msg.Images) > 0 && msg.Images[0].ImageURL.URL != "" {
		return msg.Images[0].ImageURL.URL, nil
	}

	if dataURL := extractImageURLFromText(decodeJSONString(msg.Content)); dataURL != "" {
		return dataURL, nil
	}

	var contentParts []assistantContentPart
	if err := json.Unmarshal(msg.Content, &contentParts); err == nil {
		for _, part := range contentParts {
			if dataURL := strings.TrimSpace(part.ImageURL.URL); dataURL != "" {
				return dataURL, nil
			}
			if dataURL := extractImageURLFromText(part.Text); dataURL != "" {
				return dataURL, nil
			}
		}
	}

	var contentObject assistantContentPart
	if err := json.Unmarshal(msg.Content, &contentObject); err == nil {
		if dataURL := strings.TrimSpace(contentObject.ImageURL.URL); dataURL != "" {
			return dataURL, nil
		}
		if dataURL := extractImageURLFromText(contentObject.Text); dataURL != "" {
			return dataURL, nil
		}
	}

	return "", fmt.Errorf("assistant message does not contain image data in images or content")
}

// decodeJSONString читает JSON-строку из raw content.
// Если content не строка, вернём пустое значение и перейдём к разбору других форматов.
func decodeJSONString(raw json.RawMessage) string {
	var content string
	if err := json.Unmarshal(raw, &content); err != nil {
		return ""
	}
	return content
}

// extractImageURLFromText ищет data URL или прямой URL картинки внутри текстового content.
// Некоторые провайдеры возвращают не "чистое" значение, а текст с вкраплённой ссылкой.
func extractImageURLFromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	if idx := strings.Index(text, "data:image/"); idx >= 0 {
		return strings.TrimSpace(text[idx:])
	}

	if idx := strings.Index(text, "https://"); idx >= 0 {
		candidate := strings.TrimSpace(text[idx:])
		if end := strings.IndexAny(candidate, " \r\n\t\"'"); end >= 0 {
			candidate = candidate[:end]
		}
		return candidate
	}

	return ""
}

// --- типы запроса/ответа RouterAI (OpenAI-совместимый формат) ---

type chatCompletionRequest struct {
	Model       string       `json:"model"`
	Messages    []message    `json:"messages"`
	Modalities  []string     `json:"modalities"`
	ImageConfig *imageConfig `json:"image_config,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type messagePart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
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
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
	Images  []imageItem     `json:"images"`
}

type imageItem struct {
	Type     string   `json:"type"`
	ImageURL imageURL `json:"image_url"`
}

type assistantContentPart struct {
	Type     string   `json:"type"`
	Text     string   `json:"text"`
	ImageURL imageURL `json:"image_url"`
}

type imageURL struct {
	URL string `json:"url"`
}
