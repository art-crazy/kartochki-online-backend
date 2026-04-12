-- name: CreateOAuthAccount :one
insert into oauth_accounts (
    user_id,
    provider,
    provider_user_id,
    email
) values (
    $1,
    $2,
    $3,
    $4
)
returning id, user_id, provider, provider_user_id, email, created_at;

-- name: GetOAuthAccountByProviderUserID :one
select
    id,
    user_id,
    provider,
    provider_user_id,
    email,
    created_at
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
