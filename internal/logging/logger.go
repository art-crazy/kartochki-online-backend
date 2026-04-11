package logging

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

// New создаёт zerolog.Logger с форматом, подходящим для текущего окружения.
func New(env string) zerolog.Logger {
	if env == "local" {
		return zerolog.New(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}).With().Timestamp().Logger()
	}

	return zerolog.New(os.Stdout).With().Timestamp().Logger()
}
