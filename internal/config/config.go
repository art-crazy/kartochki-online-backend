package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config объединяет конфигурацию всех подсистем приложения.
type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	Asynq    AsynqConfig
	Auth     AuthConfig
	Storage  StorageConfig
}

// AppConfig хранит общие параметры приложения.
type AppConfig struct {
	Name string
	Env  string
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
	RedirectURL  string
	StateTTL     time.Duration
}

// YandexOAuthConfig хранит env-параметры для входа через Яндекс ID.
type YandexOAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	StateTTL     time.Duration
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

	sessionTTL, err := getDuration("AUTH_SESSION_TTL", 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}

	vkStateTTL, err := getDuration("AUTH_VK_STATE_TTL", 15*time.Minute)
	if err != nil {
		return Config{}, err
	}

	yandexStateTTL, err := getDuration("AUTH_YANDEX_STATE_TTL", 15*time.Minute)
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

	emailSendTimeout, err := getDuration("AUTH_EMAIL_SEND_TIMEOUT", 30*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		App: AppConfig{
			Name: getEnv("APP_NAME", "kartochki-online-backend"),
			Env:  getEnv("APP_ENV", "local"),
		},
		HTTP: HTTPConfig{
			Host:            getEnv("HTTP_HOST", "0.0.0.0"),
			Port:            getEnv("HTTP_PORT", "8080"),
			RequestTimeout:  requestTimeout,
			ReadTimeout:     readTimeout,
			WriteTimeout:    writeTimeout,
			IdleTimeout:     idleTimeout,
			ShutdownTimeout: shutdownTimeout,
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
		Auth: AuthConfig{
			SessionTTL:            sessionTTL,
			PasswordMinLength:     passwordMinLength,
			PasswordResetTokenTTL: passwordResetTokenTTL,
			EmailSendTimeout:      emailSendTimeout,
			VKOAuth: VKOAuthConfig{
				ClientID:     getEnv("AUTH_VK_CLIENT_ID", ""),
				ClientSecret: getEnv("AUTH_VK_CLIENT_SECRET", ""),
				RedirectURL:  getEnv("AUTH_VK_REDIRECT_URL", ""),
				StateTTL:     vkStateTTL,
			},
			YandexOAuth: YandexOAuthConfig{
				ClientID:     getEnv("AUTH_YANDEX_CLIENT_ID", ""),
				ClientSecret: getEnv("AUTH_YANDEX_CLIENT_SECRET", ""),
				RedirectURL:  getEnv("AUTH_YANDEX_REDIRECT_URL", ""),
				StateTTL:     yandexStateTTL,
			},
			TelegramAuth: TelegramAuthConfig{
				BotToken:   getEnv("AUTH_TELEGRAM_BOT_TOKEN", ""),
				AuthMaxAge: telegramAuthMaxAge,
			},
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

func validate(cfg Config) error {
	if cfg.HTTP.Host == "" {
		return fmt.Errorf("HTTP_HOST must not be empty")
	}

	if cfg.HTTP.Port == "" {
		return fmt.Errorf("HTTP_PORT must not be empty")
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

	if cfg.Auth.VKOAuth.StateTTL <= 0 {
		return fmt.Errorf("AUTH_VK_STATE_TTL must be greater than zero")
	}

	if cfg.Auth.YandexOAuth.StateTTL <= 0 {
		return fmt.Errorf("AUTH_YANDEX_STATE_TTL must be greater than zero")
	}

	if cfg.Auth.TelegramAuth.AuthMaxAge <= 0 {
		return fmt.Errorf("AUTH_TELEGRAM_AUTH_MAX_AGE must be greater than zero")
	}

	return nil
}
