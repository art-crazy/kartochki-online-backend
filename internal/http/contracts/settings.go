package contracts

// SettingsResponse описывает данные для страницы `/app/settings`.
type SettingsResponse struct {
	Profile       SettingsProfile       `json:"profile"`
	Defaults      SettingsDefaults      `json:"defaults"`
	Notifications SettingsNotifications `json:"notifications"`
	Sessions      []SettingsSession     `json:"sessions"`
	Integrations  []SettingsIntegration `json:"integrations"`
	APIKey        SettingsAPIKey        `json:"api_key"`
}

// SettingsProfile описывает редактируемые данные профиля пользователя.
type SettingsProfile struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone,omitempty"`
	Company string `json:"company,omitempty"`
}

// SettingsDefaults описывает настройки генерации по умолчанию.
type SettingsDefaults struct {
	MarketplaceID      string `json:"marketplace_id"`
	CardsPerGeneration int    `json:"cards_per_generation"`
	Format             string `json:"format"`
}

// SettingsNotifications описывает набор пользовательских переключателей уведомлений.
type SettingsNotifications struct {
	Items []UpdateNotificationItem `json:"items"`
}

// SettingsSession описывает одну активную сессию пользователя.
type SettingsSession struct {
	ID        string `json:"id"`
	Device    string `json:"device"`
	Platform  string `json:"platform"`
	Location  string `json:"location,omitempty"`
	IsCurrent bool   `json:"is_current"`
	CanRevoke bool   `json:"can_revoke"`
}

// SettingsIntegration описывает один внешний подключенный аккаунт.
type SettingsIntegration struct {
	ID           string `json:"id"`
	Provider     string `json:"provider"`
	AccountEmail string `json:"account_email,omitempty"`
	Connected    bool   `json:"connected"`
}

// SettingsAPIKey описывает API-ключ интеграций в настройках пользователя.
type SettingsAPIKey struct {
	MaskedValue string `json:"masked_value"`
	CanRotate   bool   `json:"can_rotate"`
}

// UpdateProfileRequest описывает изменение профиля пользователя.
type UpdateProfileRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Phone   string `json:"phone,omitempty"`
	Company string `json:"company,omitempty"`
}

// UpdateDefaultsRequest описывает изменение настроек генерации по умолчанию.
type UpdateDefaultsRequest struct {
	MarketplaceID      string `json:"marketplace_id"`
	CardsPerGeneration int    `json:"cards_per_generation"`
	Format             string `json:"format"`
}

// ChangePasswordRequest описывает смену пароля пользователя.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UpdateNotificationsRequest описывает пакетное обновление настроек уведомлений.
type UpdateNotificationsRequest struct {
	Items []UpdateNotificationItem `json:"items"`
}

// UpdateNotificationItem описывает новое состояние одного переключателя уведомлений.
type UpdateNotificationItem struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

// RotateAPIKeyResponse возвращается после перевыпуска API-ключа.
type RotateAPIKeyResponse struct {
	MaskedValue string `json:"masked_value"`
	PlainValue  string `json:"plain_value"`
}

// ExportDataResponse подтверждает, что экспорт данных поставлен в очередь.
type ExportDataResponse struct {
	Status string `json:"status"`
}

// DeleteAccountRequest описывает подтверждение удаления аккаунта.
type DeleteAccountRequest struct {
	ConfirmWord string `json:"confirm_word"`
}
