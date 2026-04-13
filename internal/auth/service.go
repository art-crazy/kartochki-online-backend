package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"golang.org/x/oauth2"

	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/dbgen"
)

const (
	providerVK       = "vk"
	providerYandex   = "yandex"
	providerTelegram = "telegram"
)

// User описывает пользователя в auth-сценариях без HTTP-деталей.
type User struct {
	ID    string
	Name  string
	Email string
}

// Session описывает выданную клиенту локальную сессию.
type Session struct {
	AccessToken string
	ExpiresAt   time.Time
}

// SessionMetadata содержит служебные данные клиентской сессии.
// Эти поля не участвуют в авторизации, но нужны для страницы настроек и аудита устройств.
type SessionMetadata struct {
	UserAgent string
	IPAddress string
}

// AuthResult объединяет пользователя и сессию после логина или регистрации.
type AuthResult struct {
	User    User
	Session Session
}

// RegisterInput содержит данные для регистрации по email и паролю.
type RegisterInput struct {
	Name     string
	Email    string
	Password string
}

// LoginInput содержит данные для входа по email и паролю.
type LoginInput struct {
	Email    string
	Password string
}

// OAuthProviderConfig хранит данные о провайдере, которые пригодятся позже.
type OAuthProviderConfig struct {
	Name         string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	StateTTL     time.Duration
}

// TelegramAuthConfig хранит параметры серверной проверки Telegram Login Widget.
type TelegramAuthConfig struct {
	BotToken   string
	AuthMaxAge time.Duration
}

// Service хранит auth-логику и общее место расширения под OAuth.
type Service struct {
	queries               *dbgen.Queries
	pool                  *pgxpool.Pool
	stateStore            OAuthStateStore
	emailEnqueuer         PasswordResetEmailEnqueuer
	logger                zerolog.Logger
	sessionTTL            time.Duration
	passwordMinLength     int
	passwordResetTokenTTL time.Duration
	vkOAuth               OAuthProviderConfig
	yandexOAuth           OAuthProviderConfig
	telegramAuth          TelegramAuthConfig
}

// NewService создаёт auth-сервис с настройками локальных сессий и OAuth-провайдеров.
func NewService(pool *pgxpool.Pool, queries *dbgen.Queries, stateStore OAuthStateStore, emailEnqueuer PasswordResetEmailEnqueuer, logger zerolog.Logger, cfg config.AuthConfig) *Service {
	return &Service{
		queries:               queries,
		pool:                  pool,
		stateStore:            stateStore,
		emailEnqueuer:         emailEnqueuer,
		logger:                logger,
		sessionTTL:            cfg.SessionTTL,
		passwordMinLength:     cfg.PasswordMinLength,
		passwordResetTokenTTL: cfg.PasswordResetTokenTTL,
		vkOAuth: OAuthProviderConfig{
			Name:         providerVK,
			ClientID:     cfg.VKOAuth.ClientID,
			ClientSecret: cfg.VKOAuth.ClientSecret,
			RedirectURL:  cfg.VKOAuth.RedirectURL,
			StateTTL:     cfg.VKOAuth.StateTTL,
		},
		yandexOAuth: OAuthProviderConfig{
			Name:         providerYandex,
			ClientID:     cfg.YandexOAuth.ClientID,
			ClientSecret: cfg.YandexOAuth.ClientSecret,
			RedirectURL:  cfg.YandexOAuth.RedirectURL,
			StateTTL:     cfg.YandexOAuth.StateTTL,
		},
		telegramAuth: TelegramAuthConfig{
			BotToken:   cfg.TelegramAuth.BotToken,
			AuthMaxAge: cfg.TelegramAuth.AuthMaxAge,
		},
	}
}

// PasswordMinLength возвращает минимальную длину пароля для transport-валидации.
func (s *Service) PasswordMinLength() int {
	return s.passwordMinLength
}

// Register создаёт пользователя, а затем сразу выдаёт ему первую сессию.
func (s *Service) Register(ctx context.Context, input RegisterInput, metadata SessionMetadata) (AuthResult, error) {
	email := normalizeEmail(input.Email)
	if len(input.Password) < s.passwordMinLength {
		return AuthResult{}, ErrPasswordTooShort
	}

	passwordHash, err := HashPassword(input.Password)
	if err != nil {
		return AuthResult{}, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AuthResult{}, fmt.Errorf("begin auth register tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	user, err := txQueries.CreateUser(ctx, dbgen.CreateUserParams{
		Email:        pgtype.Text{String: email, Valid: true},
		PasswordHash: pgtype.Text{String: passwordHash, Valid: true},
		Name:         strings.TrimSpace(input.Name),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return AuthResult{}, ErrEmailAlreadyExists
		}

		return AuthResult{}, fmt.Errorf("create user: %w", err)
	}

	result, err := s.createSessionForUser(ctx, txQueries, User{
		ID:    user.ID.String(),
		Name:  strings.TrimSpace(user.Name),
		Email: user.Email,
	}, metadata)
	if err != nil {
		return AuthResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return AuthResult{}, fmt.Errorf("commit auth register tx: %w", err)
	}

	return result, nil
}

// Login проверяет пароль и создаёт новую сессию.
func (s *Service) Login(ctx context.Context, input LoginInput, metadata SessionMetadata) (AuthResult, error) {
	user, err := s.queries.GetLoginUserByEmail(ctx, nullableText(normalizeEmail(input.Email)))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AuthResult{}, ErrInvalidCredentials
		}

		return AuthResult{}, fmt.Errorf("get user by email: %w", err)
	}

	if user.PasswordHash == "" {
		return AuthResult{}, ErrInvalidCredentials
	}

	if err := ComparePassword(input.Password, user.PasswordHash); err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}

	return s.createSessionForUser(ctx, s.queries, User{
		ID:    user.ID.String(),
		Name:  strings.TrimSpace(user.Name),
		Email: nullableTextValue(user.Email),
	}, metadata)
}

// Authenticate находит активную сессию по Bearer-токену и возвращает её владельца.
func (s *Service) Authenticate(ctx context.Context, accessToken string) (User, error) {
	identity, err := s.queries.GetAuthIdentityByTokenHash(ctx, HashSessionToken(accessToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, ErrUnauthorized
		}

		return User{}, fmt.Errorf("get auth identity by token: %w", err)
	}

	return User{
		ID:    identity.ID.String(),
		Name:  strings.TrimSpace(identity.Name),
		Email: identity.Email,
	}, nil
}

// Logout отзывает текущую сессию по токену.
func (s *Service) Logout(ctx context.Context, accessToken string) error {
	rows, err := s.queries.RevokeSessionByTokenHash(ctx, HashSessionToken(accessToken))
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	if rows == 0 {
		return ErrUnauthorized
	}

	return nil
}

// StartVKOAuth подготавливает redirect URL на VK ID и сохраняет одноразовый state в Redis.
func (s *Service) StartVKOAuth(ctx context.Context) (string, error) {
	if !s.vkOAuthConfigured() {
		return "", ErrOAuthNotConfigured
	}

	state, err := generateOAuthState()
	if err != nil {
		return "", err
	}

	if err := s.stateStore.SaveOAuthState(ctx, oauthStateKey(s.vkOAuth.Name, state), s.vkOAuth.StateTTL); err != nil {
		return "", fmt.Errorf("save vk oauth state: %w", err)
	}

	redirectURL := newVKOAuthClient(s.vkOAuth).AuthCodeURL(state, oauth2.AccessTypeOffline)
	return redirectURL, nil
}

// FinishVKOAuth завершает внешний вход через VK ID и создаёт обычную локальную сессию backend.
func (s *Service) FinishVKOAuth(ctx context.Context, code string, state string, metadata SessionMetadata) (AuthResult, error) {
	if !s.vkOAuthConfigured() {
		return AuthResult{}, ErrOAuthNotConfigured
	}

	ok, err := s.stateStore.ConsumeOAuthState(ctx, oauthStateKey(s.vkOAuth.Name, strings.TrimSpace(state)))
	if err != nil {
		return AuthResult{}, fmt.Errorf("consume vk oauth state: %w", err)
	}

	if !ok {
		return AuthResult{}, ErrInvalidOAuthState
	}

	profile, err := fetchVKOAuthProfile(ctx, newVKOAuthClient(s.vkOAuth), strings.TrimSpace(code))
	if err != nil {
		return AuthResult{}, err
	}

	if profile.Email == "" {
		return AuthResult{}, ErrOAuthEmailMissing
	}

	return s.loginOrCreateVKOAuthUser(ctx, profile, metadata)
}

// StartYandexOAuth подготавливает redirect URL на Яндекс ID и сохраняет одноразовый state в Redis.
func (s *Service) StartYandexOAuth(ctx context.Context) (string, error) {
	if !s.providerConfigured(s.yandexOAuth) {
		return "", ErrOAuthNotConfigured
	}

	state, err := generateOAuthState()
	if err != nil {
		return "", err
	}

	if err := s.stateStore.SaveOAuthState(ctx, oauthStateKey(s.yandexOAuth.Name, state), s.yandexOAuth.StateTTL); err != nil {
		return "", fmt.Errorf("save yandex oauth state: %w", err)
	}

	redirectURL := newYandexOAuthClient(s.yandexOAuth).AuthCodeURL(state)
	return redirectURL, nil
}

// FinishYandexOAuth завершает внешний вход через Яндекс ID и создаёт обычную локальную сессию backend.
func (s *Service) FinishYandexOAuth(ctx context.Context, code string, state string, metadata SessionMetadata) (AuthResult, error) {
	if !s.providerConfigured(s.yandexOAuth) {
		return AuthResult{}, ErrOAuthNotConfigured
	}

	ok, err := s.stateStore.ConsumeOAuthState(ctx, oauthStateKey(s.yandexOAuth.Name, strings.TrimSpace(state)))
	if err != nil {
		return AuthResult{}, fmt.Errorf("consume yandex oauth state: %w", err)
	}

	if !ok {
		return AuthResult{}, ErrInvalidOAuthState
	}

	profile, err := fetchYandexOAuthProfile(ctx, newYandexOAuthClient(s.yandexOAuth), strings.TrimSpace(code))
	if err != nil {
		return AuthResult{}, err
	}

	if profile.DefaultEmail == "" {
		return AuthResult{}, ErrOAuthEmailMissing
	}

	return s.loginOrCreateOAuthUser(ctx, s.yandexOAuth.Name, profile.Subject, profile.DefaultEmail, profile.RealName, metadata)
}

// LoginWithYandexToken принимает access token от виджета Яндекс ID и создаёт локальную сессию backend.
func (s *Service) LoginWithYandexToken(ctx context.Context, accessToken string, metadata SessionMetadata) (AuthResult, error) {
	if !s.yandexTokenConfigured() {
		return AuthResult{}, ErrOAuthNotConfigured
	}

	profile, err := fetchYandexOAuthProfileByToken(ctx, strings.TrimSpace(accessToken))
	if err != nil {
		return AuthResult{}, err
	}

	if profile.DefaultEmail == "" {
		return AuthResult{}, ErrOAuthEmailMissing
	}

	return s.loginOrCreateOAuthUser(ctx, s.yandexOAuth.Name, profile.Subject, profile.DefaultEmail, profile.RealName, metadata)
}

// LoginWithTelegram проверяет подпись Telegram Login Widget и открывает локальную сессию.
func (s *Service) LoginWithTelegram(ctx context.Context, data TelegramLoginData, metadata SessionMetadata) (AuthResult, error) {
	if !s.telegramAuthConfigured() {
		return AuthResult{}, ErrTelegramAuthNotConfigured
	}

	if err := VerifyTelegramLogin(data, s.telegramAuth.BotToken, s.telegramAuth.AuthMaxAge, time.Now()); err != nil {
		return AuthResult{}, err
	}

	return s.loginOrCreateOAuthUser(ctx, providerTelegram, fmt.Sprintf("%d", data.ID), "", BuildTelegramDisplayName(data), metadata)
}

// ForgotPassword создаёт токен сброса пароля и запрашивает его отправку на email.
//
// Если пользователь с таким email не найден, метод всё равно возвращает nil,
// чтобы не раскрывать факт существования аккаунта (timing-safe ответ).
// Все предыдущие активные токены пользователя инвалидируются в той же транзакции,
// чтобы в каждый момент существовал только один рабочий токен.
func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email = normalizeEmail(email)

	user, err := s.queries.GetAuthUserByEmail(ctx, nullableText(email))
	if err != nil {
		// Пользователь не найден — тихо выходим, не раскрываем факт существования.
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}

		return fmt.Errorf("get user by email for password reset: %w", err)
	}

	// Генерируем сырой токен — он уйдёт по email и больше не появится в системе.
	rawToken, err := GenerateSessionToken()
	if err != nil {
		return fmt.Errorf("generate password reset token: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin forgot password tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	// Инвалидируем старые токены до создания нового, чтобы не накапливать рабочие ссылки.
	if err := txQueries.InvalidatePreviousPasswordResetTokens(ctx, user.ID); err != nil {
		return fmt.Errorf("invalidate previous reset tokens: %w", err)
	}

	expiresAt := time.Now().UTC().Add(s.passwordResetTokenTTL)
	_, err = txQueries.CreatePasswordResetToken(ctx, dbgen.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: HashSessionToken(rawToken),
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit forgot password tx: %w", err)
	}

	// Отправка письма — side effect после коммита транзакции.
	// Ставим задачу в очередь Asynq: это освобождает HTTP-запрос немедленно
	// и даёт worker-у несколько попыток при временном сбое SMTP.
	// Используем context.Background(), а не ctx запроса: клиент может закрыть соединение
	// сразу после ответа, и тогда ctx будет отменён раньше, чем задача встанет в очередь.
	// Ошибка постановки в очередь логируется, но не возвращается клиенту:
	// ответ одинаков независимо от результата, чтобы не раскрывать внутреннее состояние.
	if err := s.emailEnqueuer.EnqueuePasswordResetEmail(context.Background(), user.ID.String(), email, rawToken); err != nil {
		s.logger.Error().Err(err).
			Str("user_id", user.ID.String()).
			Str("email", email).
			Msg("не удалось поставить письмо сброса пароля в очередь")
	}

	return nil
}

// ResetPassword проверяет токен сброса, обновляет пароль и отзывает токен.
func (s *Service) ResetPassword(ctx context.Context, rawToken string, newPassword string) error {
	if len(newPassword) < s.passwordMinLength {
		return ErrPasswordTooShort
	}

	tokenHash := HashSessionToken(rawToken)

	// Транзакция нужна для атомарного обновления пароля и пометки токена.
	// GetValidPasswordResetToken использует FOR UPDATE — это блокирует строку токена
	// на уровне строки и не даёт параллельному запросу прочитать тот же токен
	// как активный до завершения нашей транзакции.
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin reset password tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	tokenRow, err := txQueries.GetValidPasswordResetToken(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPasswordResetTokenInvalid
		}

		return fmt.Errorf("get password reset token: %w", err)
	}

	newHash, err := HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}

	// Обновляем пароль пользователя напрямую через SQL.
	rows, err := txQueries.UpdateUserPassword(ctx, dbgen.UpdateUserPasswordParams{
		ID:           tokenRow.UserID,
		PasswordHash: pgtype.Text{String: newHash, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}

	if rows == 0 {
		// Теоретически невозможно: токен ссылается на user_id с ON DELETE CASCADE.
		return ErrUserNotFound
	}

	// Отзываем все активные сессии пользователя: после смены пароля старые токены
	// не должны давать доступ — это стандартная мера при компрометации аккаунта.
	if err := txQueries.RevokeAllUserSessions(ctx, tokenRow.UserID); err != nil {
		return fmt.Errorf("revoke user sessions after password reset: %w", err)
	}

	// Помечаем токен использованным атомарно внутри транзакции.
	// Если параллельный запрос успел пометить токен первым — получим 0 строк,
	// что означает гонку на одном токене: возвращаем ошибку невалидного токена.
	markedRows, err := txQueries.MarkPasswordResetTokenUsed(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("mark password reset token used: %w", err)
	}
	if markedRows == 0 {
		return ErrPasswordResetTokenInvalid
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reset password tx: %w", err)
	}

	return nil
}

func (s *Service) createSessionForUser(ctx context.Context, queries *dbgen.Queries, user User, metadata SessionMetadata) (AuthResult, error) {
	accessToken, err := GenerateSessionToken()
	if err != nil {
		return AuthResult{}, fmt.Errorf("generate session token: %w", err)
	}

	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		return AuthResult{}, fmt.Errorf("parse user id: %w", err)
	}

	_, err = queries.CreateSession(ctx, dbgen.CreateSessionParams{
		UserID:    userID,
		TokenHash: HashSessionToken(accessToken),
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		UserAgent: strings.TrimSpace(metadata.UserAgent),
		IpAddress: strings.TrimSpace(metadata.IPAddress),
	})
	if err != nil {
		return AuthResult{}, fmt.Errorf("create session: %w", err)
	}

	return AuthResult{
		User: user,
		Session: Session{
			AccessToken: accessToken,
			ExpiresAt:   expiresAt,
		},
	}, nil
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func (s *Service) vkOAuthConfigured() bool {
	return s.providerConfigured(s.vkOAuth)
}

func (s *Service) loginOrCreateVKOAuthUser(ctx context.Context, profile VKOAuthProfile, metadata SessionMetadata) (AuthResult, error) {
	return s.loginOrCreateOAuthUser(ctx, s.vkOAuth.Name, profile.Subject, profile.Email, profile.Name, metadata)
}

func (s *Service) providerConfigured(provider OAuthProviderConfig) bool {
	return s.stateStore != nil &&
		strings.TrimSpace(provider.ClientID) != "" &&
		strings.TrimSpace(provider.ClientSecret) != "" &&
		isRedirectURLConfigured(provider.RedirectURL)
}

func (s *Service) yandexTokenConfigured() bool {
	// Для виджета достаточно client_id, redirect_url и client_secret не используются.
	return strings.TrimSpace(s.yandexOAuth.ClientID) != ""
}

func (s *Service) telegramAuthConfigured() bool {
	return strings.TrimSpace(s.telegramAuth.BotToken) != ""
}

func (s *Service) loginOrCreateOAuthUser(ctx context.Context, providerName string, providerUserID string, email string, name string, metadata SessionMetadata) (AuthResult, error) {
	email = normalizeEmail(email)

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return AuthResult{}, fmt.Errorf("begin oauth tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	existingIdentity, err := txQueries.GetOAuthIdentityByProviderUserID(ctx, dbgen.GetOAuthIdentityByProviderUserIDParams{
		Provider:       providerName,
		ProviderUserID: providerUserID,
	})
	switch {
	case err == nil:
		result, sessionErr := s.createSessionForUser(ctx, txQueries, User{
			ID:    existingIdentity.ID.String(),
			Name:  strings.TrimSpace(existingIdentity.Name),
			Email: existingIdentity.Email,
		}, metadata)
		if sessionErr != nil {
			return AuthResult{}, sessionErr
		}

		if err := tx.Commit(ctx); err != nil {
			return AuthResult{}, fmt.Errorf("commit oauth tx: %w", err)
		}

		return result, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return AuthResult{}, fmt.Errorf("get oauth identity: %w", err)
	}

	if email != "" {
		user, err := txQueries.GetAuthUserByEmail(ctx, nullableText(email))
		switch {
		case err == nil:
			if _, err := txQueries.CreateOAuthAccount(ctx, dbgen.CreateOAuthAccountParams{
				UserID:         user.ID,
				Provider:       providerName,
				ProviderUserID: providerUserID,
				Email:          nullableText(email),
			}); err != nil {
				if !isUniqueViolation(err) {
					return AuthResult{}, fmt.Errorf("link oauth account: %w", err)
				}
			}

			result, sessionErr := s.createSessionForUser(ctx, txQueries, User{
				ID:    user.ID.String(),
				Name:  strings.TrimSpace(user.Name),
				Email: user.Email,
			}, metadata)
			if sessionErr != nil {
				return AuthResult{}, sessionErr
			}

			if err := tx.Commit(ctx); err != nil {
				return AuthResult{}, fmt.Errorf("commit oauth tx: %w", err)
			}

			return result, nil
		case !errors.Is(err, pgx.ErrNoRows):
			return AuthResult{}, fmt.Errorf("get user by email for oauth: %w", err)
		}
	}

	createdUser, err := txQueries.CreateUser(ctx, dbgen.CreateUserParams{
		Email:        nullableText(email),
		PasswordHash: pgtype.Text{},
		Name:         strings.TrimSpace(name),
	})
	if err != nil {
		return AuthResult{}, fmt.Errorf("create user from oauth: %w", err)
	}

	if _, err := txQueries.CreateOAuthAccount(ctx, dbgen.CreateOAuthAccountParams{
		UserID:         createdUser.ID,
		Provider:       providerName,
		ProviderUserID: providerUserID,
		Email:          nullableText(email),
	}); err != nil {
		return AuthResult{}, fmt.Errorf("create oauth account: %w", err)
	}

	result, err := s.createSessionForUser(ctx, txQueries, User{
		ID:    createdUser.ID.String(),
		Name:  strings.TrimSpace(createdUser.Name),
		Email: createdUser.Email,
	}, metadata)
	if err != nil {
		return AuthResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return AuthResult{}, fmt.Errorf("commit oauth tx: %w", err)
	}

	return result, nil
}

func nullableText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}

	return pgtype.Text{String: value, Valid: true}
}

func nullableTextValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}

	return strings.TrimSpace(value.String)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505"
}
