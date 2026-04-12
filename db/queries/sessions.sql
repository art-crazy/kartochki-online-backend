-- name: CreateSession :one
insert into sessions (
    user_id,
    token_hash,
    expires_at,
    user_agent,
    ip_address
) values (
    $1,
    $2,
    $3,
    $4,
    $5
)
returning id, user_id, expires_at, created_at, user_agent, ip_address;

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

-- name: RevokeOtherUserSessions :exec
-- После смены пароля в настройках оставляем текущую сессию живой,
-- чтобы пользователь не вылетал из интерфейса сразу после успешного подтверждения пароля.
update sessions
set revoked_at = now()
where user_id = $1
  and token_hash <> $2
  and revoked_at is null;

-- name: ListActiveUserSessions :many
select
    id,
    user_agent,
    ip_address,
    created_at,
    expires_at,
    token_hash = $2 as is_current
from sessions
where user_id = $1
  and revoked_at is null
  and expires_at > now()
order by created_at desc;

-- name: RevokeUserSessionByID :execrows
update sessions
set revoked_at = now()
where id = $1
  and user_id = $2
  and revoked_at is null
  and expires_at > now();
