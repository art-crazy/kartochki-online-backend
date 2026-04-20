-- name: GetUserSettingsByUserID :one
select
    user_id,
    phone,
    company,
    avatar_asset_id,
    default_marketplace,
    cards_per_generation,
    image_format,
    created_at,
    updated_at
from user_settings
where user_id = $1
limit 1;

-- name: UpsertUserSettings :one
insert into user_settings (
    user_id,
    phone,
    company,
    avatar_asset_id,
    default_marketplace,
    cards_per_generation,
    image_format
) values (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7
)
on conflict (user_id) do update
set phone = excluded.phone,
    company = excluded.company,
    avatar_asset_id = excluded.avatar_asset_id,
    default_marketplace = excluded.default_marketplace,
    cards_per_generation = excluded.cards_per_generation,
    image_format = excluded.image_format,
    updated_at = now()
returning user_id, phone, company, avatar_asset_id, default_marketplace, cards_per_generation, image_format, created_at, updated_at;

-- name: SetUserAvatarAssetID :exec
insert into user_settings (
    user_id,
    avatar_asset_id
) values (
    $1,
    $2
)
on conflict (user_id) do update
set avatar_asset_id = excluded.avatar_asset_id,
    updated_at = now();

-- name: GetUserAvatarAssetByUserID :one
select
    a.id,
    a.user_id,
    a.kind,
    a.storage_key,
    a.original_filename,
    a.mime_type,
    a.size_bytes,
    a.created_at
from user_settings us
join assets a on a.id = us.avatar_asset_id
where us.user_id = $1
limit 1;

-- name: ListNotificationPreferencesByUserID :many
select
    user_id,
    preference_key,
    enabled,
    created_at,
    updated_at
from notification_preferences
where user_id = $1
order by preference_key;

-- name: UpsertNotificationPreference :one
insert into notification_preferences (
    user_id,
    preference_key,
    enabled
) values (
    $1,
    $2,
    $3
)
on conflict (user_id, preference_key) do update
set enabled = excluded.enabled,
    updated_at = now()
returning user_id, preference_key, enabled, created_at, updated_at;

-- name: GetActiveAPIKeyByUserID :one
select
    id,
    user_id,
    key_hash,
    masked_value,
    created_at,
    last_used_at,
    revoked_at
from api_keys
where user_id = $1
  and revoked_at is null
order by created_at desc
limit 1;

-- name: RevokeActiveAPIKeysByUserID :exec
update api_keys
set revoked_at = now()
where user_id = $1
  and revoked_at is null;

-- name: CreateAPIKey :one
insert into api_keys (
    user_id,
    key_hash,
    masked_value
) values (
    $1,
    $2,
    $3
)
returning id, user_id, key_hash, masked_value, created_at, last_used_at, revoked_at;
