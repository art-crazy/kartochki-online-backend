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
