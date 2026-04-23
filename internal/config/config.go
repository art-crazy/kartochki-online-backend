package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config объединяет конфигурацию всех подсистем приложения.
type Config struct {
	App           AppConfig
	HTTP          HTTPConfig
	Postgres      PostgresConfig
	Redis         RedisConfig
	Asynq         AsynqConfig
	Auth          AuthConfig
	Storage       StorageConfig
	Email         EmailConfig
	YooKassa      YooKassaConfig
	RouterAI      RouterAIConfig
	AuthRateLimit RateLimitConfig
}

// RateLimitConfig описывает параметры ограничителя запросов (token bucket per IP).
// Применяется к публичным auth-эндпоинтам для защиты от brute force и спама.
type RateLimitConfig struct {
	// RPS — разрешённое число запросов в секунду с одного IP.
	RPS float64
	// Burst — максимальный пиковый всплеск запросов (ёмкость бакета).
	Burst int
	// CleanupTTL — через какое время неактивный IP удаляется из памяти.
	CleanupTTL time.Duration
}

// RouterAIConfig хранит параметры для интеграции с RouterAI API.
// При пустом APIKey приложение использует noopImageGenerator: генерация будет падать в failed.
// Модель не хранится здесь — пользователь выбирает её при каждой генерации.
type RouterAIConfig struct {
	APIKey   string
	Endpoint string
	Timeout  time.Duration
}

// YooKassaConfig хранит параметры для интеграции с платёжной системой ЮКасса.
// При пустом ShopID и SecretKey приложение использует noopCheckoutProvider (без реальных платежей).
type YooKassaConfig struct {
	ShopID        string
	SecretKey     string
	WebhookSecret string
	ReturnURL     string
}

// AppConfig хранит общие параметры приложения.
type AppConfig struct {
	Name         string
	Env          string
	FrontendURL  string
	CookieDomain string
}

// AuthCookieConfig описывает, как backend должен выставлять auth cookie.
type AuthCookieConfig struct {
	Domain string
	Secure bool
}

// IsProduction возвращает true, если приложение запущено в production-окружении.
// Используется для включения флагов безопасности, например Secure на куках.
func (c AppConfig) IsProduction() bool {
	return c.Env == "production"
}

// AuthCookieDomain возвращает домен, на который нужно ставить auth cookie.
// Для localhost и IP возвращаем пустую строку, чтобы браузер создал host-only cookie.
func (c AppConfig) AuthCookieDomain() string {
	domain := normalizeCookieDomain(c.CookieDomain)
	if isLocalCookieHost(domain) {
		return ""
	}
	return domain
}

// AuthCookieConfig возвращает итоговые параметры auth cookie для HTTP-слоя.
// В local по http нельзя принудительно включать Secure, иначе браузер не пришлёт cookie обратно.
func (c AppConfig) AuthCookieConfig() AuthCookieConfig {
	return AuthCookieConfig{
		Domain: c.AuthCookieDomain(),
		Secure: isHTTPSURL(c.FrontendURL),
	}
}

// EmailConfig хранит настройки SMTP-отправителя писем.
// При пустом Host приложение использует NoopSender (только логирование).
// ReplyTo необязателен: если пуст, заголовок Reply-To не добавляется.
type EmailConfig struct {
	Host        string
	Port        int
	User        string
	Password    string
	FromAddress string
	FromName    string
	ReplyTo     string
}

// HTTPConfig описывает настройки HTTP-сервера.
type HTTPConfig struct {
	Host            string
	Port            string
	RequestTimeout  time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	// CORSAllowedOrigins — список origin фронтенда без wildcard, потому что credentials: true не работает с *.
	CORSAllowedOrigins []string
}

// PostgresConfig хранит настройки подключения к PostgreSQL.
type PostgresConfig struct {
	DSN string
}

// RedisConfig хранит настройки подключения к Redis.
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// AsynqConfig хранит настройки клиента очередей Asynq.
type AsynqConfig struct {
	Concurrency int
}

// StorageConfig хранит настройки локального файлового хранилища generation-артефактов.
type StorageConfig struct {
	RootDir    string
	PublicPath string
}

// AuthConfig хранит параметры локальной авторизации и OAuth-провайдеров.
type AuthConfig struct {
	SessionTTL            time.Duration
	PasswordMinLength     int
	PasswordResetTokenTTL time.Duration
	EmailSendTimeout      time.Duration
	VKOAuth               VKOAuthConfig
	YandexOAuth           YandexOAuthConfig
	TelegramAuth          TelegramAuthConfig
}

// VKOAuthConfig хранит env-параметры для входа через VK ID.
type VKOAuthConfig struct {
	ClientID     string
	ClientSecret string
}

// YandexOAuthConfig хранит env-параметры для входа через Яндекс ID.
type YandexOAuthConfig struct {
	ClientID     string
	ClientSecret string
}

// TelegramAuthConfig хранит env-параметры для входа через Telegram Login Widget.
type TelegramAuthConfig struct {
	BotToken   string
	AuthMaxAge time.Duration
}

// Load читает конфигурацию из env и возвращает ошибку, если значения невалидны.
func Load() (Config, error) {
	_ = godotenv.Load()

	return loadFromEnv()
}

func loadFromEnv() (Config, error) {
	redisDB, err := getInt("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	smtpPort, err := getInt("SMTP_PORT", 465)
	if err != nil {
		return Config{}, err
	}

	asynqConcurrency, err := getInt("ASYNQ_CONCURRENCY", 10)
	if err != nil {
		return Config{}, err
	}

	passwordMinLength, err := getInt("AUTH_PASSWORD_MIN_LENGTH", 8)
	if err != nil {
		return Config{}, err
	}

	requestTimeout, err := getDuration("HTTP_REQUEST_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	readTimeout, err := getDuration("HTTP_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	writeTimeout, err := getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	idleTimeout, err := getDuration("HTTP_IDLE_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	sessionTTL, err := getDuration("AUTH_SESSION_TTL", 90*24*time.Hour)
	if err != nil {
		return Config{}, err
	}

	telegramAuthMaxAge, err := getDuration("AUTH_TELEGRAM_AUTH_MAX_AGE", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}

	passwordResetTokenTTL, err := getDuration("AUTH_PASSWORD_RESET_TOKEN_TTL", 1*time.Hour)
	if err != nil {
		return Config{}, err
	}

	// Даём SMTP чуть больше времени, потому что TLS-коннект и DNS у провайдера
	// могут кратковременно флапать даже при рабочем канале.
	emailSendTimeout, err := getDuration("AUTH_EMAIL_SEND_TIMEOUT", 45*time.Second)
	if err != nil {
		return Config{}, err
	}

	routerAITimeout, err := getDuration("ROUTERAI_TIMEOUT", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}

	authRateLimitBurst, err := getInt("AUTH_RATE_LIMIT_BURST", 10)
	if err != nil {
		return Config{}, err
	}

	authRateLimitCleanupTTL, err := getDuration("AUTH_RATE_LIMIT_CLEANUP_TTL", 10*time.Minute)
	if err != nil {
		return Config{}, err
	}

	authRateLimitRPS, err := getFloat("AUTH_RATE_LIMIT_RPS", 0.5)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		App: AppConfig{
			Name:         getEnv("APP_NAME", "kartochki-online-backend"),
			Env:          getEnv("APP_ENV", "local"),
			FrontendURL:  getEnv("FRONTEND_URL", "http://localhost:3000"),
			CookieDomain: normalizeCookieDomain(getEnv("AUTH_COOKIE_DOMAIN", deriveCookieDomain(getEnv("FRONTEND_URL", "http://localhost:3000")))),
		},
		HTTP: HTTPConfig{
			Host:               getEnv("HTTP_HOST", "0.0.0.0"),
			Port:               getEnv("HTTP_PORT", "8080"),
			RequestTimeout:     requestTimeout,
			ReadTimeout:        readTimeout,
			WriteTimeout:       writeTimeout,
			IdleTimeout:        idleTimeout,
			ShutdownTimeout:    shutdownTimeout,
			CORSAllowedOrigins: splitCSV(getEnv("AUTH_ALLOWED_ORIGINS", getEnv("CORS_ALLOWED_ORIGIN", "http://localhost:3000"))),
		},
		Postgres: PostgresConfig{
			DSN: getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/kartochki_online?sslmode=disable"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       redisDB,
		},
		Asynq: AsynqConfig{
			Concurrency: asynqConcurrency,
		},
		Storage: StorageConfig{
			RootDir:    getEnv("STORAGE_ROOT_DIR", "./storage"),
			PublicPath: normalizePublicPath(getEnv("STORAGE_PUBLIC_PATH", "/media")),
		},
		Email: EmailConfig{
			Host:        getEnv("SMTP_HOST", ""),
			Port:        smtpPort,
			User:        getEnv("SMTP_USER", ""),
			Password:    getEnv("SMTP_PASSWORD", ""),
			FromAddress: getEnv("EMAIL_FROM", ""),
			FromName:    getEnv("EMAIL_FROM_NAME", "kartochki.online"),
			ReplyTo:     getEnv("EMAIL_REPLY_TO", ""),
		},
		YooKassa: YooKassaConfig{
			ShopID:        getEnv("YOOKASSA_SHOP_ID", ""),
			SecretKey:     getEnv("YOOKASSA_SECRET_KEY", ""),
			WebhookSecret: getEnv("YOOKASSA_WEBHOOK_SECRET", ""),
			ReturnURL:     getEnv("YOOKASSA_RETURN_URL", "http://localhost:3000/app/billing"),
		},
		RouterAI: RouterAIConfig{
			APIKey:   getEnv("ROUTERAI_API_KEY", ""),
			Endpoint: getEnv("ROUTERAI_ENDPOINT", "https://routerai.ru/api/v1"),
			Timeout:  routerAITimeout,
		},
		Auth: AuthConfig{
			SessionTTL:            sessionTTL,
			PasswordMinLength:     passwordMinLength,
			PasswordResetTokenTTL: passwordResetTokenTTL,
			EmailSendTimeout:      emailSendTimeout,
			VKOAuth: VKOAuthConfig{
				ClientID:     getEnv("VK_ID_APP_ID", ""),
				ClientSecret: getEnv("VK_ID_CLIENT_SECRET", ""),
			},
			YandexOAuth: YandexOAuthConfig{
				ClientID:     getEnv("YANDEX_CLIENT_ID", ""),
				ClientSecret: getEnv("YANDEX_CLIENT_SECRET", ""),
			},
			TelegramAuth: TelegramAuthConfig{
				BotToken:   getEnv("AUTH_TELEGRAM_BOT_TOKEN", ""),
				AuthMaxAge: telegramAuthMaxAge,
			},
		},
		AuthRateLimit: RateLimitConfig{
			RPS:        authRateLimitRPS,
			Burst:      authRateLimitBurst,
			CleanupTTL: authRateLimitCleanupTTL,
		},
	}

	if err := validate(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func getInt(key string, fallback int) (int, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s as int: %w", key, err)
	}

	return parsed, nil
}

func getFloat(key string, fallback float64) (float64, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s as float: %w", key, err)
	}

	return parsed, nil
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s as duration: %w", key, err)
	}

	return parsed, nil
}

func normalizePublicPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "/media"
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if value != "/" {
		value = strings.TrimRight(value, "/")
	}
	return value
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}

	return result
}

func validate(cfg Config) error {
	if cfg.HTTP.Host == "" {
		return fmt.Errorf("HTTP_HOST must not be empty")
	}

	if cfg.HTTP.Port == "" {
		return fmt.Errorf("HTTP_PORT must not be empty")
	}

	if len(cfg.HTTP.CORSAllowedOrigins) == 0 {
		return fmt.Errorf("AUTH_ALLOWED_ORIGINS must contain at least one origin")
	}

	// VK ID redirect_uri в production привязан к FRONTEND_URL, поэтому здесь нужен строгий origin без path/query.
	if cfg.App.IsProduction() && !isHTTPSOrigin(cfg.App.FrontendURL) {
		return fmt.Errorf("FRONTEND_URL must be an https origin in production")
	}

	if cfg.App.IsProduction() && cfg.App.AuthCookieDomain() == "" {
		return fmt.Errorf("AUTH_COOKIE_DOMAIN must not be empty in production")
	}

	if rawCookieDomain := normalizeCookieDomain(cfg.App.CookieDomain); rawCookieDomain != "" && !isLocalCookieHost(rawCookieDomain) && !isCookieDomain(rawCookieDomain) {
		return fmt.Errorf("AUTH_COOKIE_DOMAIN must be a plain hostname without scheme, path, or port")
	}

	if cfg.Postgres.DSN == "" {
		return fmt.Errorf("POSTGRES_DSN must not be empty")
	}

	if cfg.Redis.Addr == "" {
		return fmt.Errorf("REDIS_ADDR must not be empty")
	}

	if cfg.HTTP.RequestTimeout <= 0 {
		return fmt.Errorf("HTTP_REQUEST_TIMEOUT must be greater than zero")
	}

	if cfg.HTTP.ReadTimeout <= 0 {
		return fmt.Errorf("HTTP_READ_TIMEOUT must be greater than zero")
	}

	if cfg.HTTP.WriteTimeout <= 0 {
		return fmt.Errorf("HTTP_WRITE_TIMEOUT must be greater than zero")
	}

	if cfg.HTTP.IdleTimeout <= 0 {
		return fmt.Errorf("HTTP_IDLE_TIMEOUT must be greater than zero")
	}

	if cfg.HTTP.ShutdownTimeout <= 0 {
		return fmt.Errorf("HTTP_SHUTDOWN_TIMEOUT must be greater than zero")
	}

	if cfg.Asynq.Concurrency <= 0 {
		return fmt.Errorf("ASYNQ_CONCURRENCY must be greater than zero")
	}

	if cfg.Storage.RootDir == "" {
		return fmt.Errorf("STORAGE_ROOT_DIR must not be empty")
	}

	if cfg.Storage.PublicPath == "" {
		return fmt.Errorf("STORAGE_PUBLIC_PATH must not be empty")
	}

	if cfg.Storage.PublicPath == "/" {
		return fmt.Errorf("STORAGE_PUBLIC_PATH must not be /")
	}

	if cfg.Redis.DB < 0 {
		return fmt.Errorf("REDIS_DB must not be negative")
	}

	if cfg.Auth.SessionTTL <= 0 {
		return fmt.Errorf("AUTH_SESSION_TTL must be greater than zero")
	}

	if cfg.Auth.PasswordMinLength <= 0 {
		return fmt.Errorf("AUTH_PASSWORD_MIN_LENGTH must be greater than zero")
	}

	if cfg.Auth.PasswordResetTokenTTL <= 0 {
		return fmt.Errorf("AUTH_PASSWORD_RESET_TOKEN_TTL must be greater than zero")
	}

	if cfg.Auth.EmailSendTimeout <= 0 {
		return fmt.Errorf("AUTH_EMAIL_SEND_TIMEOUT must be greater than zero")
	}

	if cfg.Auth.TelegramAuth.AuthMaxAge <= 0 {
		return fmt.Errorf("AUTH_TELEGRAM_AUTH_MAX_AGE must be greater than zero")
	}

	if cfg.AuthRateLimit.RPS <= 0 {
		return fmt.Errorf("AUTH_RATE_LIMIT_RPS must be greater than zero")
	}

	if cfg.AuthRateLimit.Burst <= 0 {
		return fmt.Errorf("AUTH_RATE_LIMIT_BURST must be greater than zero")
	}

	if cfg.AuthRateLimit.CleanupTTL <= 0 {
		return fmt.Errorf("AUTH_RATE_LIMIT_CLEANUP_TTL must be greater than zero")
	}

	// Валидируем Email только если SMTP включён — при пустом Host используется NoopSender.
	if cfg.Email.Host != "" {
		if cfg.Email.Port <= 0 {
			return fmt.Errorf("SMTP_PORT must be greater than zero when SMTP_HOST is set")
		}
		if cfg.Email.FromAddress == "" {
			return fmt.Errorf("EMAIL_FROM must not be empty when SMTP_HOST is set")
		}
	}

	return nil
}

func isHTTPSOrigin(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}

	return parsed.Scheme == "https" && parsed.Host != "" && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == ""
}

func isHTTPSURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}

	return parsed.Scheme == "https"
}

func deriveCookieDomain(frontendURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(frontendURL))
	if err != nil {
		return ""
	}

	return normalizeCookieDomain(parsed.Hostname())
}

func normalizeCookieDomain(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, ".")
	value = strings.TrimSuffix(value, ".")
	return strings.ToLower(value)
}

func isCookieDomain(value string) bool {
	if value == "" {
		return false
	}

	if strings.Contains(value, "/") || strings.Contains(value, ":") || strings.Contains(value, "://") {
		return false
	}

	return value == strings.TrimSpace(value)
}

func isLocalCookieHost(value string) bool {
	if value == "" || value == "localhost" {
		return true
	}

	parsedIP := net.ParseIP(value)
	return parsedIP != nil
}
