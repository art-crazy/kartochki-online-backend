create extension if not exists pgcrypto;

-- users хранит основной аккаунт пользователя.
-- Пароль может быть null, потому что позже пользователь сможет жить только на OAuth.
create table users (
    id uuid primary key default gen_random_uuid(),
    email varchar not null unique,
    password_hash varchar,
    name varchar not null default '',
    email_verified_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

comment on table users is 'Основной аккаунт пользователя. Не зависит от способа входа.';

-- sessions хранит только хэш access token.
-- Сырой токен отдаётся клиенту один раз и потом не хранится в базе.
create table sessions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    token_hash varchar not null unique,
    expires_at timestamptz not null,
    revoked_at timestamptz,
    created_at timestamptz not null default now(),
    constraint sessions_expires_at_check check (expires_at > created_at)
);

comment on table sessions is 'Сессии входа. В базе хранится только хэш токена.';

-- oauth_accounts заранее вводится как привязка внешнего провайдера к users.
-- Таблица нужна как общая модель для VK, Яндекса и других OAuth-провайдеров.
create table oauth_accounts (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    provider varchar not null,
    provider_user_id varchar not null,
    email varchar,
    created_at timestamptz not null default now(),
    unique (provider, provider_user_id)
);

comment on table oauth_accounts is 'Связь локального пользователя с внешним OAuth-провайдером.';

create index sessions_user_id_idx on sessions (user_id);
create index sessions_expires_at_idx on sessions (expires_at);
create index oauth_accounts_user_id_idx on oauth_accounts (user_id);
