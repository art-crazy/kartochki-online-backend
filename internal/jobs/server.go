package jobs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"
)

// GenerationHandler описывает минимальный контракт worker-обработчика generation-задачи.
type GenerationHandler interface {
	HandleGeneration(ctx context.Context, payload GenerationPayload) error
}

// SendPasswordResetEmailHandler описывает минимальный контракт worker-обработчика отправки письма сброса пароля.
type SendPasswordResetEmailHandler interface {
	HandleSendPasswordResetEmail(ctx context.Context, payload SendPasswordResetEmailPayload) error
}

// Server запускает Asynq worker в том же процессе, что и HTTP API.
type Server struct {
	server      *asynq.Server
	mux         *asynq.ServeMux
	logger      zerolog.Logger
	concurrency int
}

// NewServer создаёт worker и регистрирует известные task handlers.
func NewServer(redisOpts asynq.RedisConnOpt, concurrency int, logger zerolog.Logger, generationHandler GenerationHandler, emailHandler SendPasswordResetEmailHandler) *Server {
	mux := asynq.NewServeMux()
	if generationHandler != nil {
		mux.HandleFunc(taskTypeGeneration, func(ctx context.Context, task *asynq.Task) error {
			var payload GenerationPayload
			if err := json.Unmarshal(task.Payload(), &payload); err != nil {
				return fmt.Errorf("decode generation task payload: %w", err)
			}

			return generationHandler.HandleGeneration(ctx, payload)
		})
	}

	if emailHandler != nil {
		mux.HandleFunc(taskTypeSendPasswordResetEmail, func(ctx context.Context, task *asynq.Task) error {
			var payload SendPasswordResetEmailPayload
			if err := json.Unmarshal(task.Payload(), &payload); err != nil {
				return fmt.Errorf("decode send-password-reset-email task payload: %w", err)
			}

			return emailHandler.HandleSendPasswordResetEmail(ctx, payload)
		})
	}

	return &Server{
		server:      asynq.NewServer(redisOpts, asynq.Config{Concurrency: concurrency}),
		mux:         mux,
		logger:      logger,
		concurrency: concurrency,
	}
}

// Run запускает worker и блокируется до остановки или ошибки.
func (s *Server) Run() error {
	if s == nil || s.server == nil || s.mux == nil {
		return nil
	}

	s.logger.Info().Int("concurrency", s.concurrency).Msg("starting asynq worker")
	return s.server.Run(s.mux)
}

// Shutdown останавливает worker и даёт текущим задачам завершиться корректно.
func (s *Server) Shutdown() {
	if s == nil || s.server == nil {
		return
	}

	s.server.Shutdown()
}
