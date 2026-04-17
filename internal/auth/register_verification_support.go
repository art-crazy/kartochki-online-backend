package auth

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"kartochki-online-backend/internal/dbgen"
)

const (
	registrationVerificationCodeLength = 6
	registrationVerificationTTL        = 10 * time.Minute
	registrationResendCooldown         = 60 * time.Second
	registrationMaxVerifyAttempts      = 5
	registrationMaxResends             = 5
	registrationStartWindow            = time.Hour
	registrationMaxStartsPerEmail      = 5
	registrationMaxStartsPerIP         = 10

	pendingRegistrationStatusPending  = "pending"
	pendingRegistrationStatusVerified = "verified"
	pendingRegistrationStatusExpired  = "expired"
	pendingRegistrationStatusBlocked  = "blocked"
)

// ErrRegistrationRateLimited возвращается, когда запуск регистрации упёрся в лимит по email или IP.
var ErrRegistrationRateLimited = errors.New("registration rate limited")

// ErrVerificationNotFound возвращается, когда pending-flow не найден.
var ErrVerificationNotFound = errors.New("registration verification not found")

// ErrVerificationCodeInvalid возвращается при неверном коде подтверждения.
var ErrVerificationCodeInvalid = errors.New("registration verification code is invalid")

// ErrVerificationCodeExpired возвращается, когда код уже истёк и flow нужно начать заново.
var ErrVerificationCodeExpired = errors.New("registration verification code expired")

// ErrVerificationAttemptsExceeded возвращается, когда лимит попыток ввода кода исчерпан.
var ErrVerificationAttemptsExceeded = errors.New("registration verification attempts exceeded")

// ErrRegistrationResendTooEarly возвращается, когда до следующей отправки ещё не дошёл таймер.
var ErrRegistrationResendTooEarly = errors.New("registration resend is not available yet")

// ErrRegistrationResendLimitExceeded возвращается, когда исчерпан лимит отправок для одного flow.
var ErrRegistrationResendLimitExceeded = errors.New("registration resend limit exceeded")

// ErrVerificationAlreadyCompleted возвращается, когда flow уже успешно завершён.
var ErrVerificationAlreadyCompleted = errors.New("registration verification already completed")

type registerPendingFlowInput struct {
	Email              string
	Name               string
	PasswordHash       string
	VerificationCode   string
	VerificationExpire time.Time
	ResendAvailableAt  time.Time
	LastSentAt         time.Time
	IPAddress          string
}

func (s *Service) registerPendingFlow(ctx context.Context, txQueries *dbgen.Queries, input registerPendingFlowInput) (dbgen.PendingRegistration, bool, error) {
	pending, err := txQueries.GetPendingRegistrationByEmailForUpdate(ctx, input.Email)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.PendingRegistration{}, false, fmt.Errorf("get pending registration by email: %w", err)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		if err := s.ensureRegistrationStartAllowed(ctx, input.Email, input.IPAddress); err != nil {
			return dbgen.PendingRegistration{}, false, err
		}

		created, err := txQueries.CreatePendingRegistration(ctx, dbgen.CreatePendingRegistrationParams{
			Email:                 input.Email,
			Name:                  input.Name,
			PasswordHash:          input.PasswordHash,
			VerificationCodeHash:  input.VerificationCode,
			VerificationExpiresAt: timestamptz(input.VerificationExpire),
			ResendAvailableAt:     timestamptz(input.ResendAvailableAt),
			LastEmailSentAt:       timestamptz(input.LastSentAt),
			CreatedIpAddress:      input.IPAddress,
		})
		if err != nil {
			return dbgen.PendingRegistration{}, false, fmt.Errorf("create pending registration: %w", err)
		}
		return created, true, nil
	}

	if pending.Status == pendingRegistrationStatusVerified {
		return dbgen.PendingRegistration{}, false, ErrEmailAlreadyExists
	}

	if pending.Status == pendingRegistrationStatusPending &&
		pending.VerificationExpiresAt.Valid &&
		time.Now().UTC().Before(pending.VerificationExpiresAt.Time) {
		updated, err := txQueries.UpdatePendingRegistrationProfile(ctx, dbgen.UpdatePendingRegistrationProfileParams{
			ID:           pending.ID,
			Name:         input.Name,
			PasswordHash: input.PasswordHash,
		})
		if err != nil {
			return dbgen.PendingRegistration{}, false, fmt.Errorf("update pending registration profile: %w", err)
		}
		return updated, false, nil
	}

	if err := s.ensureRegistrationStartAllowed(ctx, input.Email, input.IPAddress); err != nil {
		return dbgen.PendingRegistration{}, false, err
	}

	refreshed, err := txQueries.RefreshPendingRegistrationCode(ctx, dbgen.RefreshPendingRegistrationCodeParams{
		ID:                    pending.ID,
		Name:                  input.Name,
		PasswordHash:          input.PasswordHash,
		VerificationCodeHash:  input.VerificationCode,
		VerificationExpiresAt: timestamptz(input.VerificationExpire),
		ResendAvailableAt:     timestamptz(input.ResendAvailableAt),
		LastEmailSentAt:       timestamptz(input.LastSentAt),
		CreatedIpAddress:      input.IPAddress,
	})
	if err != nil {
		return dbgen.PendingRegistration{}, false, fmt.Errorf("refresh pending registration code: %w", err)
	}

	return refreshed, true, nil
}

func (s *Service) ensureRegistrationStartAllowed(ctx context.Context, email string, ipAddress string) error {
	since := timestamptz(time.Now().UTC().Add(-registrationStartWindow))

	emailCount, err := s.queries.CountRecentPendingRegistrationsByEmail(ctx, dbgen.CountRecentPendingRegistrationsByEmailParams{
		Email:     email,
		CreatedAt: since,
	})
	if err != nil {
		return fmt.Errorf("count recent pending registrations by email: %w", err)
	}
	if emailCount >= registrationMaxStartsPerEmail {
		return ErrRegistrationRateLimited
	}

	if ipAddress == "" {
		return nil
	}

	ipCount, err := s.queries.CountRecentPendingRegistrationsByIP(ctx, dbgen.CountRecentPendingRegistrationsByIPParams{
		CreatedIpAddress: ipAddress,
		CreatedAt:        since,
	})
	if err != nil {
		return fmt.Errorf("count recent pending registrations by ip: %w", err)
	}
	if ipCount >= registrationMaxStartsPerIP {
		return ErrRegistrationRateLimited
	}

	return nil
}

// sendRegistrationVerificationEmail ставит письмо в очередь вместе с verification_id.
// Это позволяет письму содержать рабочую fallback-ссылку, а не только декоративный код.
func (s *Service) sendRegistrationVerificationEmail(ctx context.Context, email string, verificationID string, code string) error {
	if s.emailEnqueuer == nil {
		return fmt.Errorf("registration email enqueuer is not configured")
	}

	if err := s.emailEnqueuer.EnqueueRegistrationVerificationEmail(ctx, email, verificationID, code, registrationVerificationTTL); err != nil {
		return fmt.Errorf("enqueue registration verification email: %w", err)
	}

	return nil
}

func validatePendingRegistrationForVerification(ctx context.Context, txQueries *dbgen.Queries, pending dbgen.PendingRegistration, now time.Time) error {
	switch pending.Status {
	case pendingRegistrationStatusVerified:
		return ErrVerificationAlreadyCompleted
	case pendingRegistrationStatusExpired:
		return ErrVerificationCodeExpired
	case pendingRegistrationStatusBlocked:
		return ErrVerificationAttemptsExceeded
	}

	if pending.VerificationExpiresAt.Valid && now.After(pending.VerificationExpiresAt.Time) {
		if _, err := txQueries.ExpirePendingRegistration(ctx, pending.ID); err != nil {
			return fmt.Errorf("expire pending registration after ttl: %w", err)
		}
		return ErrVerificationCodeExpired
	}

	if pending.AttemptCount >= registrationMaxVerifyAttempts {
		if _, err := txQueries.BlockPendingRegistration(ctx, pending.ID); err != nil {
			return fmt.Errorf("block pending registration after attempts check: %w", err)
		}
		return ErrVerificationAttemptsExceeded
	}

	return nil
}

func validatePendingRegistrationForResend(ctx context.Context, txQueries *dbgen.Queries, pending dbgen.PendingRegistration, now time.Time) error {
	switch pending.Status {
	case pendingRegistrationStatusVerified:
		return ErrVerificationAlreadyCompleted
	case pendingRegistrationStatusExpired, pendingRegistrationStatusBlocked:
		return ErrVerificationCodeExpired
	}

	if pending.VerificationExpiresAt.Valid && now.After(pending.VerificationExpiresAt.Time) {
		if _, err := txQueries.ExpirePendingRegistration(ctx, pending.ID); err != nil {
			return fmt.Errorf("expire pending registration before resend: %w", err)
		}
		return ErrVerificationCodeExpired
	}

	return nil
}

func buildRegisterResult(pending dbgen.PendingRegistration, now time.Time) RegisterResult {
	return RegisterResult{
		Status:                   "pending_verification",
		VerificationID:           pending.ID.String(),
		Email:                    pending.Email,
		CodeLength:               registrationVerificationCodeLength,
		ResendAvailableInSeconds: secondsUntil(pending.ResendAvailableAt, now),
		ExpiresInSeconds:         secondsUntil(pending.VerificationExpiresAt, now),
	}
}

func secondsUntil(value pgtype.Timestamptz, now time.Time) int {
	if !value.Valid {
		return 0
	}
	seconds := int(value.Time.Sub(now).Seconds())
	if seconds < 0 {
		return 0
	}
	return seconds
}

func timestamptz(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func generateVerificationCode() (string, string, error) {
	var builder strings.Builder
	builder.Grow(registrationVerificationCodeLength)

	for i := 0; i < registrationVerificationCodeLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", "", err
		}
		builder.WriteByte(byte('0' + n.Int64()))
	}

	code := builder.String()
	return code, HashSessionToken(code), nil
}

func (s *Service) beginPendingRegistrationTx(ctx context.Context, rawVerificationID string, action string) (pgx.Tx, *dbgen.Queries, dbgen.PendingRegistration, error) {
	verificationID, err := uuid.Parse(strings.TrimSpace(rawVerificationID))
	if err != nil {
		return nil, nil, dbgen.PendingRegistration{}, ErrVerificationNotFound
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, dbgen.PendingRegistration{}, fmt.Errorf("begin %s registration tx: %w", action, err)
	}

	txQueries := s.queries.WithTx(tx)
	pending, err := txQueries.GetPendingRegistrationForUpdate(ctx, verificationID)
	if err != nil {
		_ = tx.Rollback(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, dbgen.PendingRegistration{}, ErrVerificationNotFound
		}
		return nil, nil, dbgen.PendingRegistration{}, fmt.Errorf("get pending registration for %s: %w", action, err)
	}

	return tx, txQueries, pending, nil
}
