package routerai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

const maxRetryCount = 2

var retryBackoff = []time.Duration{
	2 * time.Second,
	5 * time.Second,
}

// doChatCompletionRequest отправляет запрос к RouterAI.
// Повторяем только timeout и краткие сетевые сбои, чтобы не дублировать
// запросы после валидного ответа провайдера или бизнес-ошибки модели.
func (c *Client) doChatCompletionRequest(ctx context.Context, body []byte) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= maxRetryCount; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create routerai request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(req)
		if err == nil {
			return resp, nil
		}

		lastErr = err
		if !shouldRetryRequest(ctx, err) || attempt == maxRetryCount {
			break
		}

		// Ждём чуть дольше на каждом повторе, чтобы пережить краткий флап сети
		// или медленный холодный старт модели у провайдера.
		if waitErr := waitRetryBackoff(ctx, attempt); waitErr != nil {
			return nil, fmt.Errorf("routerai request failed: %w", waitErr)
		}
	}

	return nil, fmt.Errorf("routerai request failed: %w", lastErr)
}

func shouldRetryRequest(ctx context.Context, err error) bool {
	if err == nil {
		return false
	}

	// Если родительский контекст уже отменён приложением, повторять нельзя.
	if ctx.Err() != nil && !errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
}

func waitRetryBackoff(ctx context.Context, attempt int) error {
	delay := retryBackoff[min(attempt, len(retryBackoff)-1)]
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
