package config

import (
	"fmt"
	"os"
	"strconv"
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

// Load читает конфигурацию из env и возвращает ошибку, если значения невалидны.
//
// Для production безопаснее упасть на старте, чем молча принять неправильный
// timeout или номер Redis DB и получить трудноуловимое поведение позже.
func Load() (Config, error) {
	_ = godotenv.Load()

	return loadFromEnv()
}

// loadFromEnv собирает конфигурацию из уже доступных env-переменных.
func loadFromEnv() (Config, error) {
	redisDB, err := getInt("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}

	asynqConcurrency, err := getInt("ASYNQ_CONCURRENCY", 10)
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

// getInt читает целочисленную env-переменную.
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

// getDuration читает env-переменную с Go duration-форматом.
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

// validate проверяет базовые инварианты конфигурации, важные для старта сервиса.
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

	if cfg.Redis.DB < 0 {
		return fmt.Errorf("REDIS_DB must not be negative")
	}

	return nil
}
