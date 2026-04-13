package http

import (
	"net"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/http/response"
)

// ipLimiter хранит rate limiter для одного IP-адреса и время последнего обращения.
// lastSeen хранится как unix-наносекунды через atomic, чтобы обновлять его
// без эксклюзивной блокировки карты и избежать data race.
type ipLimiter struct {
	limiter  *rate.Limiter
	lastSeen atomic.Int64 // unix nano
}

// rateLimiter ограничивает количество запросов с одного IP на заданную группу эндпоинтов.
// Очистка устаревших записей происходит в фоне, чтобы не накапливать память.
type rateLimiter struct {
	// RWMutex защищает только карту limiters; само поле lastSeen обновляется атомарно.
	mu       sync.RWMutex
	limiters map[string]*ipLimiter

	// rps и burst нужны для создания новых ipLimiter при первом обращении IP.
	rps   rate.Limit
	burst int

	ttl        time.Duration
	retryAfter string // предвычислено из rps, не меняется после создания
}

// newRateLimiter создаёт лимитер с заданными параметрами и запускает фоновую очистку.
func newRateLimiter(cfg config.RateLimitConfig) *rateLimiter {
	rl := &rateLimiter{
		limiters:   make(map[string]*ipLimiter),
		rps:        rate.Limit(cfg.RPS),
		burst:      cfg.Burst,
		ttl:        cfg.CleanupTTL,
		retryAfter: strconv.Itoa(int(1.0/cfg.RPS) + 1),
	}

	// Фоновая горутина удаляет устаревшие записи каждые CleanupTTL, чтобы избежать
	// неограниченного роста карты при большом числе уникальных IP.
	go rl.cleanupLoop(cfg.CleanupTTL)

	return rl
}

// getLimiter возвращает существующий или новый limiter для данного IP
// и атомарно обновляет время последнего обращения.
func (rl *rateLimiter) getLimiter(ip string) *rate.Limiter {
	// Оптимистичный путь под RLock: карта не меняется, lastSeen обновляем атомарно.
	rl.mu.RLock()
	entry, ok := rl.limiters[ip]
	rl.mu.RUnlock()

	if ok {
		entry.lastSeen.Store(time.Now().UnixNano())
		return entry.limiter
	}

	// IP встречается впервые — берём эксклюзивную блокировку для записи в карту.
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check: другая горутина могла создать запись пока мы ждали Lock.
	if entry, ok = rl.limiters[ip]; ok {
		entry.lastSeen.Store(time.Now().UnixNano())
		return entry.limiter
	}

	entry = &ipLimiter{
		limiter: rate.NewLimiter(rl.rps, rl.burst),
	}
	entry.lastSeen.Store(time.Now().UnixNano())
	rl.limiters[ip] = entry

	return entry.limiter
}

// cleanupLoop периодически удаляет IP, которые не обращались дольше ttl.
func (rl *rateLimiter) cleanupLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		threshold := time.Now().Add(-rl.ttl).UnixNano()

		rl.mu.Lock()
		for ip, entry := range rl.limiters {
			if entry.lastSeen.Load() < threshold {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Middleware возвращает HTTP middleware, которое отклоняет запросы при превышении лимита.
func (rl *rateLimiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// RealIP middleware уже обработал X-Forwarded-For / X-Real-IP,
			// но r.RemoteAddr имеет формат "ip:port" — стрипаем порт.
			ip, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				// Fallback: если формат неожиданный, используем адрес как есть.
				ip = r.RemoteAddr
			}

			if !rl.getLimiter(ip).Allow() {
				w.Header().Set("Retry-After", rl.retryAfter)
				response.WriteError(
					w, r,
					http.StatusTooManyRequests,
					"rate_limit_exceeded",
					"слишком много запросов, попробуйте позже",
				)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
