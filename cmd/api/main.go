package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"kartochki-online-backend/internal/app"
	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/logging"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}

	logger := logging.New(cfg.App.Env)

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Fatal().Err(err).Msg("failed to build application")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)

	go func() {
		logger.Info().
			Str("addr", application.Server.Address()).
			Msg("starting http server")

		errCh <- application.Server.Start()
	}()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal().Err(err).Msg("http server stopped with error")
		}
	case <-ctx.Done():
		logger.Info().Msg("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		logger.Fatal().Err(err).Msg("graceful shutdown failed")
	}

	logger.Info().Msg("application stopped")
}
