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

// SendAuthEmailHandler описывает минимальный контракт worker-обработчика auth-писем.
type SendAuthEmailHandler interface {
	HandleSendAuthEmail(ctx context.Context, payload SendAuthEmailPayload) error
}

// BillingRenewalHandler описывает обработчик периодического автопродления подписок.
type BillingRenewalHandler interface {
	HandleBillingRenewSubscriptions(ctx context.Context, payload BillingRenewSubscriptionsPayload) error
}

// Server запускает Asynq worker в том же процессе, что и HTTP API.
type Server struct {
	server      *asynq.Server
	scheduler   *asynq.Scheduler
	mux         *asynq.ServeMux
	logger      zerolog.Logger
	concurrency int
}

// NewServer создаёт worker и регистрирует известные task handlers.
func NewServer(redisOpts asynq.RedisConnOpt, concurrency int, logger zerolog.Logger, generationHandler GenerationHandler, emailHandler SendAuthEmailHandler, billingRenewalHandler BillingRenewalHandler) *Server {
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
		mux.HandleFunc(taskTypeSendAuthEmail, func(ctx context.Context, task *asynq.Task) error {
			var payload SendAuthEmailPayload
			if err := json.Unmarshal(task.Payload(), &payload); err != nil {
				return fmt.Errorf("decode send-auth-email task payload: %w", err)
			}

			return emailHandler.HandleSendAuthEmail(ctx, payload)
		})
	}

	scheduler := asynq.NewScheduler(redisOpts, nil)
	if billingRenewalHandler != nil {
		mux.HandleFunc(taskTypeBillingRenewSubscriptions, func(ctx context.Context, task *asynq.Task) error {
			var payload BillingRenewSubscriptionsPayload
			if err := json.Unmarshal(task.Payload(), &payload); err != nil {
				return fmt.Errorf("decode billing renewal task payload: %w", err)
			}

			return billingRenewalHandler.HandleBillingRenewSubscriptions(ctx, payload)
		})

		task := asynq.NewTask(taskTypeBillingRenewSubscriptions, mustMarshal(BillingRenewSubscriptionsPayload{BatchLimit: billingRenewalBatchLimit}))
		if _, err := scheduler.Register("@every 30m", task); err != nil {
			logger.Error().Err(err).Msg("failed to register billing renewal scheduler")
		}
	}

	return &Server{
		server:      asynq.NewServer(redisOpts, asynq.Config{Concurrency: concurrency}),
		scheduler:   scheduler,
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
	if s.scheduler != nil {
		go func() {
			if err := s.scheduler.Run(); err != nil {
				s.logger.Error().Err(err).Msg("asynq scheduler stopped")
			}
		}()
	}
	return s.server.Run(s.mux)
}

// Shutdown останавливает worker и даёт текущим задачам завершиться корректно.
func (s *Server) Shutdown() {
	if s == nil || s.server == nil {
		return
	}

	if s.scheduler != nil {
		s.scheduler.Shutdown()
	}
	s.server.Shutdown()
}

func mustMarshal(payload any) []byte {
	body, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}

	return body
}
