-- name: CreateUser :one
insert into users (
    email,
    password_hash,
    name
) values (
    $1,
    $2,
    $3
)
returning id, coalesce(email, '') as email, name, created_at, updated_at;

-- name: CreateVerifiedUser :one
insert into users (
    email,
    password_hash,
    name,
    email_verified_at
) values (
    $1,
    $2,
    $3,
    now()
)
returning id, coalesce(email, '') as email, name, created_at, updated_at;

-- name: GetLoginUserByEmail :one
select
    id,
    email,
    name,
    coalesce(password_hash, '') as password_hash
from users
where email = $1
limit 1;

-- name: GetAuthUserByID :one
select
    id,
    coalesce(email, '') as email,
    name
from users
where id = $1
limit 1;

-- name: GetAuthUserByEmail :one
select
    id,
    coalesce(email, '') as email,
    name
from users
where email = $1
limit 1;

-- name: GetUserCredentialsByID :one
select
    id,
    coalesce(email, '') as email,
    name,
    coalesce(password_hash, '') as password_hash
from users
where id = $1
limit 1;

-- name: UpdateUserProfile :one
update users
set name = $2,
    email = $3,
    updated_at = now()
where id = $1
returning id, coalesce(email, '') as email, name, created_at, updated_at;

-- name: UpdateUserPassword :execrows
-- Обновляем хэш пароля пользователя. Возвращает количество затронутых строк —
-- 0 означает, что пользователь не найден.
update users
set password_hash = $2,
    updated_at = now()
where id = $1;

-- name: DeleteUserByID :execrows
delete from users
where id = $1;
