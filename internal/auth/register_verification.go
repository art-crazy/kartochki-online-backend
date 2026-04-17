package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"kartochki-online-backend/internal/dbgen"
)

// RegisterResult описывает ответ первого шага регистрации до создания сессии.
type RegisterResult struct {
	Status                   string
	VerificationID           string
	Email                    string
	CodeLength               int
	ResendAvailableInSeconds int
	ExpiresInSeconds         int
}

// VerifyRegistrationInput содержит код из письма и идентификатор pending-flow.
type VerifyRegistrationInput struct {
	VerificationID string
	Code           string
}

// ResendRegistrationCodeInput содержит идентификатор flow, для которого нужен новый код.
type ResendRegistrationCodeInput struct {
	VerificationID string
}

// Register запускает регистрацию по email/паролю и отправляет одноразовый код подтверждения.
func (s *Service) Register(ctx context.Context, input RegisterInput) (RegisterResult, error) {
	email := normalizeEmail(input.Email)
	name := strings.TrimSpace(input.Name)
	ipAddress := strings.TrimSpace(input.IPAddress)

	if len(input.Password) < s.passwordMinLength {
		return RegisterResult{}, ErrPasswordTooShort
	}

	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("hash password: %w", err)
	}

	code, codeHash, err := generateVerificationCode()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("generate verification code: %w", err)
	}

	now := time.Now().UTC()
	expiresAt := now.Add(registrationVerificationTTL)
	resendAvailableAt := now.Add(registrationResendCooldown)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RegisterResult{}, fmt.Errorf("begin register verification tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	if _, err := txQueries.GetAuthUserByEmail(ctx, nullableText(email)); err == nil {
		return RegisterResult{}, ErrEmailAlreadyExists
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return RegisterResult{}, fmt.Errorf("check existing user by email: %w", err)
	}

	pending, shouldSendCode, err := s.registerPendingFlow(ctx, txQueries, registerPendingFlowInput{
		Email:              email,
		Name:               name,
		PasswordHash:       passwordHash,
		VerificationCode:   codeHash,
		VerificationExpire: expiresAt,
		ResendAvailableAt:  resendAvailableAt,
		LastSentAt:         now,
		IPAddress:          ipAddress,
	})
	if err != nil {
		return RegisterResult{}, err
	}

	if shouldSendCode {
		// Письмо отправляем до commit, чтобы при ошибке не оставить в базе flow с кодом,
		// который пользователь никогда не получит.
		if err := s.sendRegistrationVerificationEmail(ctx, email, pending.ID.String(), code); err != nil {
			return RegisterResult{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, fmt.Errorf("commit register verification tx: %w", err)
	}

	return buildRegisterResult(pending, now), nil
}

// VerifyRegistration проверяет код, создаёт пользователя и открывает первую сессию.
func (s *Service) VerifyRegistration(ctx context.Context, input VerifyRegistrationInput, metadata SessionMetadata) (AuthResult, error) {
	tx, txQueries, pending, err := s.beginPendingRegistrationTx(ctx, input.VerificationID, "verify")
	if err != nil {
		return AuthResult{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if err := validatePendingRegistrationForVerification(ctx, txQueries, pending, now); err != nil {
		return AuthResult{}, err
	}

	if HashSessionToken(strings.TrimSpace(input.Code)) != pending.VerificationCodeHash {
		attemptCount, err := txQueries.IncrementPendingRegistrationAttempt(ctx, pending.ID)
		if err != nil {
			return AuthResult{}, fmt.Errorf("increment verification attempts: %w", err)
		}
		if attemptCount >= registrationMaxVerifyAttempts {
			if _, blockErr := txQueries.BlockPendingRegistration(ctx, pending.ID); blockErr != nil {
				return AuthResult{}, fmt.Errorf("block pending registration: %w", blockErr)
			}
			return AuthResult{}, ErrVerificationAttemptsExceeded
		}
		return AuthResult{}, ErrVerificationCodeInvalid
	}

	createdUser, err := txQueries.CreateVerifiedUser(ctx, dbgen.CreateVerifiedUserParams{
		Email:        pgtype.Text{String: pending.Email, Valid: true},
		PasswordHash: pgtype.Text{String: pending.PasswordHash, Valid: true},
		Name:         pending.Name,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return AuthResult{}, ErrEmailAlreadyExists
		}
		return AuthResult{}, fmt.Errorf("create verified user: %w", err)
	}

	result, err := s.createSessionForUser(ctx, txQueries, User{
		ID:    createdUser.ID.String(),
		Name:  strings.TrimSpace(createdUser.Name),
		Email: createdUser.Email,
	}, metadata)
	if err != nil {
		return AuthResult{}, err
	}

	if _, err := txQueries.CompletePendingRegistration(ctx, pending.ID); err != nil {
		return AuthResult{}, fmt.Errorf("complete pending registration: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return AuthResult{}, fmt.Errorf("commit verify registration tx: %w", err)
	}

	return result, nil
}

// ResendRegistrationCode выпускает новый код для действующего flow и отправляет его повторно.
func (s *Service) ResendRegistrationCode(ctx context.Context, input ResendRegistrationCodeInput) (RegisterResult, error) {
	tx, txQueries, pending, err := s.beginPendingRegistrationTx(ctx, input.VerificationID, "resend")
	if err != nil {
		return RegisterResult{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	if err := validatePendingRegistrationForResend(ctx, txQueries, pending, now); err != nil {
		return RegisterResult{}, err
	}

	if pending.ResendAvailableAt.Valid && now.Before(pending.ResendAvailableAt.Time) {
		return RegisterResult{}, ErrRegistrationResendTooEarly
	}
	if pending.ResendCount >= registrationMaxResends {
		if _, err := txQueries.ExpirePendingRegistration(ctx, pending.ID); err != nil {
			return RegisterResult{}, fmt.Errorf("expire pending registration after resend limit: %w", err)
		}
		return RegisterResult{}, ErrRegistrationResendLimitExceeded
	}

	code, codeHash, err := generateVerificationCode()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("generate verification code: %w", err)
	}

	updated, err := txQueries.ResendPendingRegistrationCode(ctx, dbgen.ResendPendingRegistrationCodeParams{
		ID:                    pending.ID,
		VerificationCodeHash:  codeHash,
		VerificationExpiresAt: timestamptz(now.Add(registrationVerificationTTL)),
		ResendAvailableAt:     timestamptz(now.Add(registrationResendCooldown)),
		ResendCount:           pending.ResendCount + 1,
		LastEmailSentAt:       timestamptz(now),
	})
	if err != nil {
		return RegisterResult{}, fmt.Errorf("resend pending registration code: %w", err)
	}

	// Новый код становится активным только вместе с успешной отправкой письма.
	if err := s.sendRegistrationVerificationEmail(ctx, updated.Email, updated.ID.String(), code); err != nil {
		return RegisterResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, fmt.Errorf("commit resend verification tx: %w", err)
	}

	return buildRegisterResult(updated, now), nil
}
