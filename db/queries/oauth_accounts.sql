-- name: CreateOAuthAccount :one
insert into oauth_accounts (
    user_id,
    provider,
    provider_user_id,
    email,
    name,
    avatar_url
) values (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6
) on conflict (provider, provider_user_id) do nothing
returning id, user_id, provider, provider_user_id, email, name, avatar_url, created_at, updated_at;

-- name: GetOAuthAccountByProviderUserID :one
select
    id,
    user_id,
    provider,
    provider_user_id,
    email,
    name,
    avatar_url,
    created_at,
    updated_at
from oauth_accounts
where provider = $1
  and provider_user_id = $2
limit 1;

-- name: GetOAuthIdentityByProviderUserID :one
select
    u.id,
    coalesce(u.email, '') as email,
    u.name
from oauth_accounts oa
join users u on u.id = oa.user_id
where oa.provider = $1
  and oa.provider_user_id = $2
limit 1;

-- name: UpdateOAuthAccountSnapshot :exec
update oauth_accounts
set email = coalesce($3, email),
    name = coalesce($4, name),
    avatar_url = coalesce($5, avatar_url),
    updated_at = now()
where provider = $1
  and provider_user_id = $2;

-- name: ListOAuthAccountsByUserID :many
select
    id,
    provider,
    coalesce(email, '') as email,
    created_at
from oauth_accounts
where user_id = $1
order by created_at asc;
