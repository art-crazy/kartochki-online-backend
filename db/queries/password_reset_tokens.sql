-- name: InvalidatePreviousPasswordResetTokens :exec
-- Инвалидируем все предыдущие активные токены пользователя перед созданием нового.
-- Это гарантирует, что в каждый момент у пользователя есть только один рабочий токен,
-- и предотвращает накопление активных ссылок в базе.
update password_reset_tokens
set used_at = now()
where user_id = $1
  and used_at is null
  and expires_at > now();

-- name: CreatePasswordResetToken :one
-- Создаём новый токен сброса. Предыдущие токены должны быть инвалидированы заранее
-- через InvalidatePreviousPasswordResetTokens в той же транзакции.
insert into password_reset_tokens (
    user_id,
    token_hash,
    expires_at
) values (
    $1,
    $2,
    $3
)
returning id, user_id, expires_at, created_at;

-- name: GetValidPasswordResetToken :one
-- Ищем токен по хэшу только среди активных: не истёкших и не использованных.
-- FOR UPDATE блокирует строку на уровне транзакции, чтобы предотвратить
-- одновременное использование одного токена двумя параллельными запросами.
select
    id,
    user_id
from password_reset_tokens
where token_hash = $1
  and used_at is null
  and expires_at > now()
limit 1
for update;

-- name: MarkPasswordResetTokenUsed :execrows
-- Помечаем токен использованным. Повторный вызов с тем же хэшем вернёт 0 строк.
update password_reset_tokens
set used_at = now()
where token_hash = $1
  and used_at is null;
