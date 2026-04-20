package settings

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

	"kartochki-online-backend/internal/auth"
	"kartochki-online-backend/internal/dbgen"
	"kartochki-online-backend/internal/jobs"
	"kartochki-online-backend/internal/platform/storage"
)

const (
	defaultCardsPerGeneration = 10
	defaultImageFormat        = "png"
	deleteAccountConfirmWord  = "DELETE"
	apiKeyPrefix              = "ko_live_"
)

var defaultNotificationItems = []NotificationItem{
	{Key: "billing_updates", Enabled: true},
	{Key: "generation_ready", Enabled: true},
	{Key: "product_updates", Enabled: false},
}

// Settings описывает данные страницы `/app/settings` без HTTP-деталей.
type Settings struct {
	Profile       Profile
	Defaults      Defaults
	Notifications []NotificationItem
	Sessions      []Session
	Integrations  []Integration
	APIKey        APIKey
}

// Profile описывает редактируемые поля профиля пользователя.
type Profile struct {
	Name          string
	Email         string
	EmailVerified bool
	Phone         string
	Company       string
}

// Defaults описывает значения по умолчанию для будущих генераций.
type Defaults struct {
	MarketplaceID      string
	CardsPerGeneration int
	Format             string
}

// NotificationItem описывает один переключатель уведомлений.
type NotificationItem struct {
	Key     string
	Enabled bool
}

// Session описывает одну активную пользовательскую сессию.
type Session struct {
	ID        string
	Device    string
	Platform  string
	Location  string
	IsCurrent bool
	CanRevoke bool
}

// Integration описывает подключённого внешнего провайдера входа.
type Integration struct {
	ID           string
	Provider     string
	AccountEmail string
	Connected    bool
}

// APIKey описывает текущий активный API-ключ пользователя.
type APIKey struct {
	MaskedValue string
	CanRotate   bool
}

// RotatedAPIKey содержит новый API-ключ сразу после ротации.
// PlainValue возвращается только один раз, поэтому frontend должен показать его сразу после обновления.
type RotatedAPIKey struct {
	MaskedValue string
	PlainValue  string
}

// UpdateProfileInput описывает сохранение профиля.
type UpdateProfileInput struct {
	Name    string
	Email   string
	Phone   string
	Company string
}

// UpdateDefaultsInput описывает сохранение настроек генерации по умолчанию.
type UpdateDefaultsInput struct {
	MarketplaceID      string
	CardsPerGeneration int
	Format             string
}

type avatarStorage interface {
	Save(ctx context.Context, storageKey string, body []byte) (storage.SavedFile, error)
	Delete(ctx context.Context, storageKey string) error
	PublicURL(storageKey string) string
}

// Service управляет пользовательскими настройками и связанными security-сценариями.
type Service struct {
	pool              *pgxpool.Pool
	queries           *dbgen.Queries
	jobsClient        *jobs.Client
	storage           avatarStorage
	passwordMinLength int
}

// NewService создаёт сервис настроек поверх sqlc-запросов и очереди фоновых задач.
func NewService(pool *pgxpool.Pool, queries *dbgen.Queries, jobsClient *jobs.Client, storage avatarStorage, passwordMinLength int) *Service {
	return &Service{
		pool:              pool,
		queries:           queries,
		jobsClient:        jobsClient,
		storage:           storage,
		passwordMinLength: passwordMinLength,
	}
}

// PasswordMinLength возвращает минимальную длину пароля для transport-валидации.
func (s *Service) PasswordMinLength() int {
	return s.passwordMinLength
}

// Get собирает данные для страницы настроек, объединяя профиль, дефолты, уведомления и security-блок.
func (s *Service) Get(ctx context.Context, userID string, currentAccessToken string) (Settings, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return Settings{}, ErrUserNotFound
	}

	user, err := s.queries.GetAuthUserByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Settings{}, ErrUserNotFound
		}

		return Settings{}, fmt.Errorf("get settings user: %w", err)
	}

	userSettings, err := s.getOrDefaultUserSettings(ctx, s.queries, uid)
	if err != nil {
		return Settings{}, err
	}

	notifications, err := s.getNotificationItems(ctx, uid)
	if err != nil {
		return Settings{}, err
	}

	sessions, err := s.getSessions(ctx, uid, currentAccessToken)
	if err != nil {
		return Settings{}, err
	}

	integrations, err := s.getIntegrations(ctx, uid)
	if err != nil {
		return Settings{}, err
	}

	apiKey, err := s.getAPIKey(ctx, uid)
	if err != nil {
		return Settings{}, err
	}

	return Settings{
		Profile: Profile{
			Name:          strings.TrimSpace(user.Name),
			Email:         strings.TrimSpace(user.Email),
			EmailVerified: isEmailVerified(user.PasswordHash, user.EmailVerifiedAt.Valid),
			Phone:         strings.TrimSpace(userSettings.Phone),
			Company:       strings.TrimSpace(userSettings.Company),
		},
		Defaults: Defaults{
			MarketplaceID:      strings.TrimSpace(userSettings.DefaultMarketplace),
			CardsPerGeneration: int(userSettings.CardsPerGeneration),
			Format:             strings.TrimSpace(userSettings.ImageFormat),
		},
		Notifications: notifications,
		Sessions:      sessions,
		Integrations:  integrations,
		APIKey:        apiKey,
	}, nil
}

// UpdateProfile сохраняет профиль в users и user_settings в одной транзакции.
func (s *Service) UpdateProfile(ctx context.Context, userID string, input UpdateProfileInput) (Profile, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return Profile{}, ErrUserNotFound
	}

	input = normalizeProfileInput(input)
	if input.Name == "" {
		return Profile{}, ErrNameRequired
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Profile{}, fmt.Errorf("begin update profile tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	existingUser, err := txQueries.GetAuthUserByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Profile{}, ErrUserNotFound
		}

		return Profile{}, fmt.Errorf("get user before profile update: %w", err)
	}

	// Если email уже подтверждён — игнорируем любые попытки его изменить через этот endpoint.
	// Смена подтверждённого email требует отдельного flow с верификацией.
	if isEmailVerified(existingUser.PasswordHash, existingUser.EmailVerifiedAt.Valid) {
		input.Email = existingUser.Email
	}

	user, err := txQueries.UpdateUserProfile(ctx, dbgen.UpdateUserProfileParams{
		ID:    uid,
		Name:  input.Name,
		Email: nullableText(input.Email),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return Profile{}, ErrEmailTaken
		}

		return Profile{}, fmt.Errorf("update user profile: %w", err)
	}

	currentSettings, err := s.getOrDefaultUserSettings(ctx, txQueries, uid)
	if err != nil {
		return Profile{}, err
	}

	settingsRow, err := txQueries.UpsertUserSettings(ctx, dbgen.UpsertUserSettingsParams{
		UserID:             uid,
		Phone:              input.Phone,
		Company:            input.Company,
		AvatarAssetID:      currentSettings.AvatarAssetID,
		DefaultMarketplace: currentSettings.DefaultMarketplace,
		CardsPerGeneration: currentSettings.CardsPerGeneration,
		ImageFormat:        currentSettings.ImageFormat,
	})
	if err != nil {
		return Profile{}, fmt.Errorf("upsert user settings for profile: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Profile{}, fmt.Errorf("commit update profile tx: %w", err)
	}

	return Profile{
		Name:          strings.TrimSpace(user.Name),
		Email:         strings.TrimSpace(user.Email),
		EmailVerified: isEmailVerified(existingUser.PasswordHash, existingUser.EmailVerifiedAt.Valid),
		Phone:         strings.TrimSpace(settingsRow.Phone),
		Company:       strings.TrimSpace(settingsRow.Company),
	}, nil
}

// UpdateDefaults сохраняет дефолтные параметры генерации.
func (s *Service) UpdateDefaults(ctx context.Context, userID string, input UpdateDefaultsInput) (Defaults, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return Defaults{}, ErrUserNotFound
	}

	input = normalizeDefaultsInput(input)
	if input.CardsPerGeneration < 1 || input.CardsPerGeneration > 50 {
		return Defaults{}, ErrCardsPerGenerationOutOfRange
	}
	if !isAllowedImageFormat(input.Format) {
		return Defaults{}, ErrInvalidImageFormat
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Defaults{}, fmt.Errorf("begin update defaults tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	if _, err := txQueries.GetAuthUserByID(ctx, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Defaults{}, ErrUserNotFound
		}

		return Defaults{}, fmt.Errorf("get user before defaults update: %w", err)
	}

	currentSettings, err := s.getOrDefaultUserSettings(ctx, txQueries, uid)
	if err != nil {
		return Defaults{}, err
	}

	row, err := txQueries.UpsertUserSettings(ctx, dbgen.UpsertUserSettingsParams{
		UserID:             uid,
		Phone:              currentSettings.Phone,
		Company:            currentSettings.Company,
		AvatarAssetID:      currentSettings.AvatarAssetID,
		DefaultMarketplace: input.MarketplaceID,
		CardsPerGeneration: int32(input.CardsPerGeneration),
		ImageFormat:        input.Format,
	})
	if err != nil {
		return Defaults{}, fmt.Errorf("upsert user settings for defaults: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Defaults{}, fmt.Errorf("commit update defaults tx: %w", err)
	}

	return Defaults{
		MarketplaceID:      strings.TrimSpace(row.DefaultMarketplace),
		CardsPerGeneration: int(row.CardsPerGeneration),
		Format:             strings.TrimSpace(row.ImageFormat),
	}, nil
}

// UpdateNotifications пакетно сохраняет переключатели уведомлений.
func (s *Service) UpdateNotifications(ctx context.Context, userID string, items []NotificationItem) ([]NotificationItem, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return nil, ErrUserNotFound
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("begin update notifications tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	if _, err := txQueries.GetAuthUserByID(ctx, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}

		return nil, fmt.Errorf("get user before notifications update: %w", err)
	}

	for _, item := range items {
		key := strings.TrimSpace(item.Key)
		if !isKnownNotificationKey(key) {
			return nil, ErrUnknownNotificationKey
		}

		if _, err := txQueries.UpsertNotificationPreference(ctx, dbgen.UpsertNotificationPreferenceParams{
			UserID:        uid,
			PreferenceKey: key,
			Enabled:       item.Enabled,
		}); err != nil {
			return nil, fmt.Errorf("upsert notification preference: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit update notifications tx: %w", err)
	}

	return s.getNotificationItems(ctx, uid)
}

// ChangePassword обновляет локальный пароль и отзывает все остальные активные сессии пользователя.
func (s *Service) ChangePassword(ctx context.Context, userID string, currentAccessToken string, currentPassword string, newPassword string) error {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return ErrUserNotFound
	}

	if len(newPassword) < s.passwordMinLength {
		return auth.ErrPasswordTooShort
	}

	user, err := s.queries.GetUserCredentialsByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}

		return fmt.Errorf("get user credentials for password change: %w", err)
	}

	if strings.TrimSpace(user.PasswordHash) == "" || auth.ComparePassword(currentPassword, user.PasswordHash) != nil {
		return ErrCurrentPasswordInvalid
	}

	newHash, err := auth.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("hash new password in settings: %w", err)
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin change password tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	rows, err := txQueries.UpdateUserPassword(ctx, dbgen.UpdateUserPasswordParams{
		ID:           uid,
		PasswordHash: nullableText(newHash),
	})
	if err != nil {
		return fmt.Errorf("update user password from settings: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	if err := txQueries.RevokeOtherUserSessions(ctx, dbgen.RevokeOtherUserSessionsParams{
		UserID:    uid,
		TokenHash: auth.HashSessionToken(currentAccessToken),
	}); err != nil {
		return fmt.Errorf("revoke other sessions after password change: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit change password tx: %w", err)
	}

	return nil
}

// DeleteSession отзывает одну пользовательскую сессию, кроме текущей.
func (s *Service) DeleteSession(ctx context.Context, userID string, sessionID string, currentAccessToken string) error {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return ErrUserNotFound
	}

	rows, err := s.queries.ListActiveUserSessions(ctx, dbgen.ListActiveUserSessionsParams{
		UserID:    uid,
		TokenHash: auth.HashSessionToken(currentAccessToken),
	})
	if err != nil {
		return fmt.Errorf("list sessions before delete: %w", err)
	}

	targetID := strings.TrimSpace(sessionID)
	for _, row := range rows {
		if row.ID.String() != targetID {
			continue
		}
		if row.IsCurrent {
			return ErrCannotRevokeCurrentSession
		}

		affected, err := s.queries.RevokeUserSessionByID(ctx, dbgen.RevokeUserSessionByIDParams{
			ID:     row.ID,
			UserID: uid,
		})
		if err != nil {
			return fmt.Errorf("revoke session by id: %w", err)
		}
		if affected == 0 {
			return ErrSessionNotFound
		}
		return nil
	}

	return ErrSessionNotFound
}

// RotateAPIKey отзывает старый активный ключ и создаёт новый.
func (s *Service) RotateAPIKey(ctx context.Context, userID string) (RotatedAPIKey, error) {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return RotatedAPIKey{}, ErrUserNotFound
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return RotatedAPIKey{}, fmt.Errorf("begin rotate api key tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)
	if _, err := txQueries.GetAuthUserByID(ctx, uid); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RotatedAPIKey{}, ErrUserNotFound
		}

		return RotatedAPIKey{}, fmt.Errorf("get user before api key rotate: %w", err)
	}

	plainToken, err := auth.GenerateSessionToken()
	if err != nil {
		return RotatedAPIKey{}, fmt.Errorf("generate api key token: %w", err)
	}
	plainValue := apiKeyPrefix + plainToken
	maskedValue := maskSecret(plainValue)

	if err := txQueries.RevokeActiveAPIKeysByUserID(ctx, uid); err != nil {
		return RotatedAPIKey{}, fmt.Errorf("revoke old api keys: %w", err)
	}

	if _, err := txQueries.CreateAPIKey(ctx, dbgen.CreateAPIKeyParams{
		UserID:      uid,
		KeyHash:     auth.HashSessionToken(plainValue),
		MaskedValue: maskedValue,
	}); err != nil {
		return RotatedAPIKey{}, fmt.Errorf("create api key: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return RotatedAPIKey{}, fmt.Errorf("commit rotate api key tx: %w", err)
	}

	return RotatedAPIKey{
		MaskedValue: maskedValue,
		PlainValue:  plainValue,
	}, nil
}

// ExportData ставит задачу экспорта аккаунта в очередь.
func (s *Service) ExportData(ctx context.Context, userID string) error {
	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return ErrUserNotFound
	}

	user, err := s.queries.GetUserCredentialsByID(ctx, uid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}

		return fmt.Errorf("get user before export enqueue: %w", err)
	}

	if s.jobsClient == nil {
		return fmt.Errorf("settings export queue is not configured")
	}

	if _, err := s.jobsClient.EnqueueExportAccountData(ctx, jobs.ExportAccountDataPayload{
		UserID:      user.ID.String(),
		UserEmail:   strings.TrimSpace(user.Email),
		RequestedAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("enqueue account export: %w", err)
	}

	return nil
}

// DeleteAccount удаляет пользователя целиком вместе с каскадно связанными данными.
// Сессии отзываются явно внутри транзакции до удаления пользователя,
// чтобы исключить race window: middleware не сможет авторизовать запрос
// по токену, который формально ещё числился активным в момент DELETE.
func (s *Service) DeleteAccount(ctx context.Context, userID string, confirmWord string) error {
	if strings.TrimSpace(confirmWord) != deleteAccountConfirmWord {
		return ErrInvalidConfirmWord
	}

	uid, err := uuid.Parse(strings.TrimSpace(userID))
	if err != nil {
		return ErrUserNotFound
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin delete account tx: %w", err)
	}
	defer tx.Rollback(ctx)

	txQueries := s.queries.WithTx(tx)

	// Сначала отзываем все сессии, пока строка пользователя ещё существует.
	// Каскад ON DELETE CASCADE всё равно удалил бы их, но явный отзыв
	// гарантирует, что токены перестают работать до удаления пользователя.
	if err := txQueries.RevokeAllUserSessions(ctx, uid); err != nil {
		return fmt.Errorf("revoke sessions before account delete: %w", err)
	}

	rows, err := txQueries.DeleteUserByID(ctx, uid)
	if err != nil {
		return fmt.Errorf("delete user account: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit delete account tx: %w", err)
	}

	return nil
}

func (s *Service) getOrDefaultUserSettings(ctx context.Context, queries settingsQueries, userID uuid.UUID) (dbgen.GetUserSettingsByUserIDRow, error) {
	row, err := queries.GetUserSettingsByUserID(ctx, userID)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return dbgen.GetUserSettingsByUserIDRow{}, fmt.Errorf("get user settings: %w", err)
	}

	return dbgen.GetUserSettingsByUserIDRow{
		UserID:             userID,
		Phone:              "",
		Company:            "",
		AvatarAssetID:      pgtype.UUID{},
		DefaultMarketplace: "",
		CardsPerGeneration: defaultCardsPerGeneration,
		ImageFormat:        defaultImageFormat,
	}, nil
}

func (s *Service) getNotificationItems(ctx context.Context, userID uuid.UUID) ([]NotificationItem, error) {
	rows, err := s.queries.ListNotificationPreferencesByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list notification preferences: %w", err)
	}

	byKey := make(map[string]bool, len(rows))
	for _, row := range rows {
		byKey[strings.TrimSpace(row.PreferenceKey)] = row.Enabled
	}

	result := make([]NotificationItem, len(defaultNotificationItems))
	for i, item := range defaultNotificationItems {
		enabled, ok := byKey[item.Key]
		if ok {
			result[i] = NotificationItem{Key: item.Key, Enabled: enabled}
			continue
		}
		result[i] = item
	}

	return result, nil
}

func (s *Service) getSessions(ctx context.Context, userID uuid.UUID, currentAccessToken string) ([]Session, error) {
	rows, err := s.queries.ListActiveUserSessions(ctx, dbgen.ListActiveUserSessionsParams{
		UserID:    userID,
		TokenHash: auth.HashSessionToken(currentAccessToken),
	})
	if err != nil {
		return nil, fmt.Errorf("list active user sessions: %w", err)
	}

	result := make([]Session, len(rows))
	for i, row := range rows {
		device, platform := describeUserAgent(row.UserAgent)
		result[i] = Session{
			ID:        row.ID.String(),
			Device:    device,
			Platform:  platform,
			Location:  strings.TrimSpace(row.IpAddress),
			IsCurrent: row.IsCurrent,
			CanRevoke: !row.IsCurrent,
		}
	}

	return result, nil
}

func (s *Service) getIntegrations(ctx context.Context, userID uuid.UUID) ([]Integration, error) {
	rows, err := s.queries.ListOAuthAccountsByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list oauth accounts for settings: %w", err)
	}

	byProvider := make(map[string]dbgen.ListOAuthAccountsByUserIDRow, len(rows))
	for _, row := range rows {
		byProvider[strings.TrimSpace(row.Provider)] = row
	}

	providers := []string{"vk", "yandex", "telegram"}
	result := make([]Integration, len(providers))
	for i, provider := range providers {
		row, ok := byProvider[provider]
		if ok {
			result[i] = Integration{
				ID:           row.ID.String(),
				Provider:     provider,
				AccountEmail: strings.TrimSpace(row.Email),
				Connected:    true,
			}
			continue
		}

		result[i] = Integration{
			Provider:  provider,
			Connected: false,
		}
	}

	return result, nil
}

func (s *Service) getAPIKey(ctx context.Context, userID uuid.UUID) (APIKey, error) {
	row, err := s.queries.GetActiveAPIKeyByUserID(ctx, userID)
	if err == nil {
		return APIKey{
			MaskedValue: strings.TrimSpace(row.MaskedValue),
			CanRotate:   true,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, fmt.Errorf("get active api key: %w", err)
	}

	return APIKey{CanRotate: true}, nil
}

func normalizeProfileInput(input UpdateProfileInput) UpdateProfileInput {
	input.Name = strings.TrimSpace(input.Name)
	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Phone = strings.TrimSpace(input.Phone)
	input.Company = strings.TrimSpace(input.Company)
	return input
}

func normalizeDefaultsInput(input UpdateDefaultsInput) UpdateDefaultsInput {
	input.MarketplaceID = strings.TrimSpace(input.MarketplaceID)
	input.Format = strings.TrimSpace(strings.ToLower(input.Format))
	return input
}

func isAllowedImageFormat(format string) bool {
	switch format {
	case "png", "jpg", "webp":
		return true
	default:
		return false
	}
}

func isKnownNotificationKey(key string) bool {
	for _, item := range defaultNotificationItems {
		if item.Key == key {
			return true
		}
	}

	return false
}

func describeUserAgent(userAgent string) (string, string) {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return "Unknown device", "Unknown platform"
	}

	browser := "Browser"
	switch {
	case strings.Contains(ua, "edg/"):
		browser = "Edge"
	case strings.Contains(ua, "chrome/") && !strings.Contains(ua, "edg/"):
		browser = "Chrome"
	case strings.Contains(ua, "safari/") && !strings.Contains(ua, "chrome/"):
		browser = "Safari"
	case strings.Contains(ua, "firefox/"):
		browser = "Firefox"
	}

	platform := "Unknown platform"
	switch {
	case strings.Contains(ua, "iphone") || strings.Contains(ua, "ipad"):
		platform = "iOS"
	case strings.Contains(ua, "android"):
		platform = "Android"
	case strings.Contains(ua, "windows"):
		platform = "Windows"
	case strings.Contains(ua, "mac os x") || strings.Contains(ua, "macintosh"):
		platform = "macOS"
	case strings.Contains(ua, "linux"):
		platform = "Linux"
	}

	device := browser + " browser"
	if strings.Contains(ua, "iphone") {
		device = browser + " on iPhone"
	} else if strings.Contains(ua, "ipad") {
		device = browser + " on iPad"
	} else if strings.Contains(ua, "android") {
		device = browser + " on Android"
	}

	return device, platform
}

func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return strings.Repeat("*", len(secret))
	}

	return secret[:4] + strings.Repeat("*", len(secret)-8) + secret[len(secret)-4:]
}

func nullableText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}

	return pgtype.Text{String: value, Valid: true}
}

// isEmailVerified определяет, является ли email подтверждённым.
// Считается подтверждённым, если пользователь зарегистрировался через email+пароль
// или вошёл через OAuth-провайдер, который вернул email (email_verified_at заполнен).
func isEmailVerified(passwordHash string, hasVerifiedAt bool) bool {
	return passwordHash != "" || hasVerifiedAt
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505"
}

type settingsQueries interface {
	GetUserSettingsByUserID(ctx context.Context, userID uuid.UUID) (dbgen.GetUserSettingsByUserIDRow, error)
}
