package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	Asynq    AsynqConfig
}

type AppConfig struct {
	Name string
	Env  string
}

type HTTPConfig struct {
	Host            string
	Port            string
	RequestTimeout  time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
}

type PostgresConfig struct {
	DSN string
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type AsynqConfig struct {
	Concurrency int
}

func Load() (Config, error) {
	_ = godotenv.Load()

	return Config{
		App: AppConfig{
			Name: getEnv("APP_NAME", "kartochki-online-backend"),
			Env:  getEnv("APP_ENV", "local"),
		},
		HTTP: HTTPConfig{
			Host:            getEnv("HTTP_HOST", "0.0.0.0"),
			Port:            getEnv("HTTP_PORT", "8080"),
			RequestTimeout:  getDuration("HTTP_REQUEST_TIMEOUT", 30*time.Second),
			ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 5*time.Second),
			WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:     getDuration("HTTP_IDLE_TIMEOUT", 30*time.Second),
			ShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
		},
		Postgres: PostgresConfig{
			DSN: getEnv("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/kartochki_online?sslmode=disable"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getInt("REDIS_DB", 0),
		},
		Asynq: AsynqConfig{
			Concurrency: getInt("ASYNQ_CONCURRENCY", 10),
		},
	}, nil
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func getInt(key string, fallback int) int {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getDuration(key string, fallback time.Duration) time.Duration {
	value := getEnv(key, "")
	if value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
