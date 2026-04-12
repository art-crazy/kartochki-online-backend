-- user_settings хранит расширенный профиль и пользовательские значения по умолчанию.
-- Выделяем их из users, чтобы auth-ядро оставалось компактным, а настройки росли отдельно.
create table user_settings (
    user_id uuid primary key references users (id) on delete cascade,
    phone varchar not null default '',
    company varchar not null default '',
    default_marketplace varchar not null default '',
    cards_per_generation integer not null default 10,
    image_format varchar not null default 'png',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint user_settings_cards_per_generation_check check (cards_per_generation between 1 and 50),
    constraint user_settings_image_format_check check (image_format in ('png', 'jpg', 'webp'))
);

comment on table user_settings is 'Расширенный профиль пользователя и дефолтные параметры генерации.';

-- notification_preferences хранит явные переключатели уведомлений.
-- Каждая настройка живёт отдельной строкой, чтобы можно было добавлять новые ключи без миграции схемы ответа.
create table notification_preferences (
    user_id uuid not null references users (id) on delete cascade,
    preference_key varchar not null,
    enabled boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    primary key (user_id, preference_key)
);

comment on table notification_preferences is 'Пользовательские переключатели уведомлений.';

-- api_keys хранит только хэш секрета.
-- В каждый момент времени у пользователя есть максимум один активный ключ, чтобы ротация была предсказуемой.
create table api_keys (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    key_hash varchar not null unique,
    masked_value varchar not null,
    created_at timestamptz not null default now(),
    last_used_at timestamptz,
    revoked_at timestamptz
);

comment on table api_keys is 'API-ключи пользовательских интеграций. Секрет хранится только в виде хэша.';

create unique index api_keys_active_user_idx
    on api_keys (user_id)
    where revoked_at is null;

-- Сохраняем базовые метаданные сессии, чтобы страница настроек могла показать список устройств.
alter table sessions
    add column user_agent text not null default '',
    add column ip_address varchar not null default '';
