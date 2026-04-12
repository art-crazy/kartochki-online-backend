-- name: CreateSession :one
insert into sessions (
    user_id,
    token_hash,
    expires_at
) values (
    $1,
    $2,
    $3
)
returning id, user_id, expires_at, created_at;

-- name: GetAuthIdentityByTokenHash :one
select
    u.id,
    coalesce(u.email, '') as email,
    u.name,
    s.id as session_id,
    s.expires_at
from sessions s
join users u on u.id = s.user_id
where s.token_hash = $1
  and s.revoked_at is null
  and s.expires_at > now()
limit 1;

-- name: RevokeSessionByTokenHash :execrows
update sessions
set revoked_at = now()
where token_hash = $1
  and revoked_at is null;

-- name: RevokeAllUserSessions :exec
-- Отзываем все активные сессии пользователя. Вызывается при смене пароля,
-- чтобы злоумышленник потерял доступ к аккаунту даже с ранее выданным токеном.
update sessions
set revoked_at = now()
where user_id = $1
  and revoked_at is null;
