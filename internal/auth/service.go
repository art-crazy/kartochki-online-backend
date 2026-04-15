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
	"kartochki-online-backend/internal/config"
	"kartochki-online-backend/internal/dbgen"
)

const (
	providerVK       = "vk"
	providerYandex   = "yandex"
	providerTelegram = "telegram"

	maxOAuthLoginRetries = 2
)

// User описывает пользователя в auth-сценариях без HTTP-деталей.
type User struct {
	ID        string
	Name      string
	Email     string
	AvatarURL string
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
}

// VKWidgetLoginInput содержит одноразовые OAuth-данные, которые frontend получил от VK ID SDK.
type VKWidgetLoginInput struct {
	Code         string
	DeviceID     string
	CodeVerifier string
	RedirectURI  string
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
func NewService(pool *pgxpool.Pool, queries *dbgen.Queries, emailEnqueuer PasswordResetEmailEnqueuer, logger zerolog.Logger, cfg config.AuthConfig) *Service {
	return &Service{
		queries:               queries,
		pool:                  pool,
		emailEnqueuer:         emailEnqueuer,
		logger:                logger,
		sessionTTL:            cfg.SessionTTL,
		passwordMinLength:     cfg.PasswordMinLength,
		passwordResetTokenTTL: cfg.PasswordResetTokenTTL,
		vkOAuth: OAuthProviderConfig{
			Name:         providerVK,
			ClientID:     cfg.VKOAuth.ClientID,
			ClientSecret: cfg.VKOAuth.ClientSecret,
		},
		yandexOAuth: OAuthProviderConfig{
			Name:         providerYandex,
			ClientID:     cfg.YandexOAuth.ClientID,
			ClientSecret: cfg.YandexOAuth.ClientSecret,
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

// WithLatestOAuthAvatar добавляет к пользователю свежий аватар из OAuth-снимка.
// Ошибка чтения не ломает ответ: аватар не участвует в авторизации и может временно отсутствовать.
func (s *Service) WithLatestOAuthAvatar(ctx context.Context, user User) User {
	userID, err := uuid.Parse(user.ID)
	if err != nil {
		s.logger.Warn().Err(err).Str("user_id", user.ID).Msg("не удалось прочитать user id для загрузки аватара")
		return user
	}

	user.AvatarURL = s.latestOAuthAvatarURL(ctx, s.queries, userID)
	return user
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

// LoginWithVKOAuth обменивает code на токен через стандартный VK OAuth 2.0 Authorization Code + PKCE flow
// и открывает локальную сессию. Device ID не требуется — это отличие от widget flow.
func (s *Service) LoginWithVKOAuth(ctx context.Context, input VKOAuthLoginInput, metadata SessionMetadata) (AuthResult, error) {
	if !s.vkOAuthConfigured() {
		return AuthResult{}, ErrOAuthNotConfigured
	}

	profile, err := fetchVKOAuthProfile(ctx, s.vkOAuth, input)
	if err != nil {
		return AuthResult{}, oauthProviderError(err)
	}

	return s.loginOrCreateVKOAuthUser(ctx, profile, metadata)
}

// LoginWithVKWidget проверяет code, device_id и PKCE verifier от VK ID One Tap и открывает локальную сессию.
// Email от VK может отсутствовать, поэтому пользователь создаётся и без email.
func (s *Service) LoginWithVKWidget(ctx context.Context, input VKWidgetLoginInput, metadata SessionMetadata) (AuthResult, error) {
	if !s.vkWidgetConfigured() {
		return AuthResult{}, ErrOAuthNotConfigured
	}

	profile, err := fetchVKWidgetProfile(ctx, s.vkOAuth, input)
	if err != nil {
		return AuthResult{}, oauthProviderError(err)
	}

	return s.loginOrCreateVKOAuthUser(ctx, profile, metadata)
}

// LoginWithYandexWidget принимает access token от виджета Яндекс ID и создаёт локальную сессию backend.
func (s *Service) LoginWithYandexWidget(ctx context.Context, accessToken string, metadata SessionMetadata) (AuthResult, error) {
	if !s.yandexTokenConfigured() {
		return AuthResult{}, ErrOAuthNotConfigured
	}

	profile, err := fetchYandexOAuthProfileByToken(ctx, strings.TrimSpace(accessToken))
	if err != nil {
		return AuthResult{}, oauthProviderError(err)
	}

	return s.loginOrCreateOAuthUser(ctx, s.yandexOAuth.Name, profile.Subject, profile.DefaultEmail, profile.RealName, yandexAvatarURL(profile.AvatarID), metadata)
}

// LoginWithTelegram проверяет подпись Telegram Login Widget и открывает локальную сессию.
func (s *Service) LoginWithTelegram(ctx context.Context, data TelegramLoginData, metadata SessionMetadata) (AuthResult, error) {
	if !s.telegramAuthConfigured() {
		return AuthResult{}, ErrTelegramAuthNotConfigured
	}

	if err := VerifyTelegramLogin(data, s.telegramAuth.BotToken, s.telegramAuth.AuthMaxAge, time.Now()); err != nil {
		return AuthResult{}, err
	}

	return s.loginOrCreateOAuthUser(ctx, providerTelegram, fmt.Sprintf("%d", data.ID), "", BuildTelegramDisplayName(data), data.PhotoURL, metadata)
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

// vkOAuthConfigured проверяет только client_id — PKCE flow не использует client_secret.
func (s *Service) vkOAuthConfigured() bool {
	return strings.TrimSpace(s.vkOAuth.ClientID) != ""
}

func (s *Service) vkWidgetConfigured() bool {
	return strings.TrimSpace(s.vkOAuth.ClientID) != "" &&
		strings.TrimSpace(s.vkOAuth.ClientSecret) != ""
}

func (s *Service) loginOrCreateVKOAuthUser(ctx context.Context, profile VKOAuthProfile, metadata SessionMetadata) (AuthResult, error) {
	return s.loginOrCreateOAuthUser(ctx, s.vkOAuth.Name, profile.Subject, profile.Email, profile.Name, profile.AvatarURL, metadata)
}

func (s *Service) yandexTokenConfigured() bool {
	return strings.TrimSpace(s.yandexOAuth.ClientID) != "" &&
		strings.TrimSpace(s.yandexOAuth.ClientSecret) != ""
}

func (s *Service) telegramAuthConfigured() bool {
	return strings.TrimSpace(s.telegramAuth.BotToken) != ""
}

func (s *Service) loginOrCreateOAuthUser(ctx context.Context, providerName string, providerUserID string, email string, name string, avatarURL string, metadata SessionMetadata) (AuthResult, error) {
	return s.loginOrCreateOAuthUserAttempt(ctx, providerName, providerUserID, email, name, avatarURL, metadata, 0)
}

func (s *Service) loginOrCreateOAuthUserAttempt(ctx context.Context, providerName string, providerUserID string, email string, name string, avatarURL string, metadata SessionMetadata, attempt int) (AuthResult, error) {
	email = normalizeEmail(email)
	providerUserID = strings.TrimSpace(providerUserID)
	if providerUserID == "" {
		return AuthResult{}, fmt.Errorf("%w: provider user id is empty", ErrOAuthProviderError)
	}

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
		// При каждом входе обновляем снимок профиля, но не стираем поля, которые провайдер не вернул.
		if err := txQueries.UpdateOAuthAccountSnapshot(ctx, dbgen.UpdateOAuthAccountSnapshotParams{
			Provider:       providerName,
			ProviderUserID: providerUserID,
			Email:          nullableText(email),
			Name:           nullableText(name),
			AvatarUrl:      nullableText(avatarURL),
		}); err != nil {
			return AuthResult{}, fmt.Errorf("update oauth account snapshot: %w", err)
		}

		result, sessionErr := s.createSessionForUser(ctx, txQueries, User{
			ID:        existingIdentity.ID.String(),
			Name:      strings.TrimSpace(existingIdentity.Name),
			Email:     existingIdentity.Email,
			AvatarURL: firstNonEmpty(avatarURL, existingIdentity.AvatarUrl),
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
				Name:           nullableText(name),
				AvatarUrl:      nullableText(avatarURL),
			}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) && attempt < maxOAuthLoginRetries {
					// В параллельном запросе такая же внешняя учётка могла привязаться первой.
					// Откатываем текущую транзакцию и повторяем вход, чтобы provider identity был источником правды.
					_ = tx.Rollback(ctx)
					return s.loginOrCreateOAuthUserAttempt(ctx, providerName, providerUserID, email, name, avatarURL, metadata, attempt+1)
				}

				return AuthResult{}, fmt.Errorf("link oauth account: %w", err)
			}

			resolvedAvatarURL := strings.TrimSpace(avatarURL)
			if resolvedAvatarURL == "" {
				resolvedAvatarURL = s.latestOAuthAvatarURL(ctx, txQueries, user.ID)
			}

			result, sessionErr := s.createSessionForUser(ctx, txQueries, User{
				ID:        user.ID.String(),
				Name:      strings.TrimSpace(user.Name),
				Email:     user.Email,
				AvatarURL: resolvedAvatarURL,
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
		if email != "" && isUniqueViolation(err) && attempt < maxOAuthLoginRetries {
			// Если такой email появился параллельно, повторный проход найдёт пользователя
			// и привяжет OAuth-аккаунт без создания дубля.
			_ = tx.Rollback(ctx)
			return s.loginOrCreateOAuthUserAttempt(ctx, providerName, providerUserID, email, name, avatarURL, metadata, attempt+1)
		}

		return AuthResult{}, fmt.Errorf("create user from oauth: %w", err)
	}

	if _, err := txQueries.CreateOAuthAccount(ctx, dbgen.CreateOAuthAccountParams{
		UserID:         createdUser.ID,
		Provider:       providerName,
		ProviderUserID: providerUserID,
		Email:          nullableText(email),
		Name:           nullableText(name),
		AvatarUrl:      nullableText(avatarURL),
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) && attempt < maxOAuthLoginRetries {
			// Если привязка появилась между поиском и insert, не коммитим нового пользователя.
			_ = tx.Rollback(ctx)
			return s.loginOrCreateOAuthUserAttempt(ctx, providerName, providerUserID, email, name, avatarURL, metadata, attempt+1)
		}

		return AuthResult{}, fmt.Errorf("create oauth account: %w", err)
	}

	result, err := s.createSessionForUser(ctx, txQueries, User{
		ID:        createdUser.ID.String(),
		Name:      strings.TrimSpace(createdUser.Name),
		Email:     createdUser.Email,
		AvatarURL: avatarURL,
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}

	return ""
}

func (s *Service) latestOAuthAvatarURL(ctx context.Context, queries *dbgen.Queries, userID uuid.UUID) string {
	avatarURL, err := queries.GetLatestOAuthAvatarByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ""
		}

		// Аватар не участвует в авторизации, поэтому сбой чтения не должен ломать текущую сессию.
		s.logger.Warn().Err(err).Str("user_id", userID.String()).Msg("не удалось загрузить OAuth-аватар пользователя")
		return ""
	}

	return strings.TrimSpace(avatarURL)
}

func oauthProviderError(err error) error {
	if errors.Is(err, ErrOAuthTokenInvalid) {
		return err
	}

	return fmt.Errorf("%w: %v", ErrOAuthProviderError, err)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505"
}
