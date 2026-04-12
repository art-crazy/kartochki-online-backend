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

type runtimeResult struct {
	component string
	err       error
}

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

	errCh := make(chan runtimeResult, 2)

	go func() {
		logger.Info().
			Str("addr", application.Server.Address()).
			Msg("starting http server")

		errCh <- runtimeResult{
			component: "http_server",
			err:       application.Server.Start(),
		}
	}()

	if application.Worker != nil {
		go func() {
			errCh <- runtimeResult{
				component: "asynq_worker",
				err:       application.Worker.Run(),
			}
		}()
	}

	select {
	case result := <-errCh:
		if result.err != nil {
			if result.component == "http_server" && errors.Is(result.err, http.ErrServerClosed) {
				break
			}

			logger.Fatal().
				Err(result.err).
				Str("component", result.component).
				Msg("runtime component stopped with error")
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
