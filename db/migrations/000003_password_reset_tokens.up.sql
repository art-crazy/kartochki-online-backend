-- password_reset_tokens хранит одноразовые токены сброса пароля.
-- В базе лежит только хэш токена — сырая ссылка уходит на email.
-- Использованный токен помечается полем used_at, а не удаляется, для аудита.
create table password_reset_tokens (
    id         uuid primary key default gen_random_uuid(),
    user_id    uuid not null references users (id) on delete cascade,
    token_hash varchar not null unique,
    expires_at timestamptz not null,
    used_at    timestamptz,
    created_at timestamptz not null default now(),
    constraint password_reset_tokens_expires_at_check
        check (expires_at > created_at)
);

comment on table password_reset_tokens is 'Одноразовые токены сброса пароля. Хранится только хэш, сырой токен уходит по email.';

create index password_reset_tokens_user_id_idx  on password_reset_tokens (user_id);
create index password_reset_tokens_expires_at_idx on password_reset_tokens (expires_at);
