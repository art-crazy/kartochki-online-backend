package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword хэширует пароль для безопасного хранения в PostgreSQL.
func HashPassword(password string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	return string(hashed), nil
}

// ComparePassword проверяет, что введённый пароль совпадает с сохранённым хэшем.
func ComparePassword(password string, passwordHash string) error {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
}
