-- Черновая схема PostgreSQL для импорта в DataGrip.
-- Это проектный DDL перед миграциями, а не финальная миграция для production.
-- Скрипт рассчитан на разворачивание в пустой базе для проектирования схемы.

create extension if not exists pgcrypto;

-- Пользователи и аутентификация.

create table users (
    id uuid primary key default gen_random_uuid(),
    email varchar not null unique,
    password_hash varchar,
    name varchar,
    email_verified_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

comment on table users is 'Основной аккаунт пользователя.';

create table user_settings (
    user_id uuid primary key
        references users (id) on delete cascade,
    default_card_count integer,
    default_language varchar,
    default_marketplace varchar,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint user_settings_default_card_count_check
        check (default_card_count is null or default_card_count > 0)
);

comment on table user_settings is 'Настройки профиля и дефолты генерации в формате 1:1 к users.';

create table sessions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    token_hash varchar not null unique,
    device varchar,
    platform varchar,
    location varchar,
    ip inet,
    user_agent text,
    last_seen_at timestamptz,
    expires_at timestamptz not null,
    revoked_at timestamptz,
    created_at timestamptz not null default now(),
    constraint sessions_expires_at_check
        check (expires_at > created_at)
);

comment on table sessions is 'Сессии входа. Для безопасности в БД хранится хэш токена.';

create table password_reset_tokens (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    token_hash varchar not null unique,
    expires_at timestamptz not null,
    used_at timestamptz,
    created_at timestamptz not null default now(),
    constraint password_reset_tokens_expires_at_check
        check (expires_at > created_at)
);

create table oauth_accounts (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    provider varchar not null,
    provider_user_id varchar not null,
    email varchar,
    created_at timestamptz not null default now(),
    unique (provider, provider_user_id)
);

comment on table oauth_accounts is 'Привязанные внешние OAuth-аккаунты.';

create table notification_preferences (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    kind varchar not null,
    enabled boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (user_id, kind)
);

comment on table notification_preferences is 'Переключатели уведомлений по ключам, а не отдельными колонками.';

create table api_keys (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    key_prefix varchar not null,
    key_hash varchar not null unique,
    is_active boolean not null default true,
    last_used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz not null default now()
);

comment on table api_keys is 'API-ключи пользователя с историей перевыпуска.';

-- Биллинг.

create table plans (
    id uuid primary key default gen_random_uuid(),
    code varchar not null unique,
    name varchar not null,
    period varchar not null,
    cards_limit integer not null,
    price_amount numeric(12, 2) not null,
    price_currency varchar not null,
    is_active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint plans_period_check
        check (period in ('monthly', 'yearly')),
    constraint plans_cards_limit_check
        check (cards_limit >= 0),
    constraint plans_price_amount_check
        check (price_amount >= 0)
);

create table addon_products (
    id uuid primary key default gen_random_uuid(),
    code varchar not null unique,
    name varchar not null,
    cards_count integer not null,
    price_amount numeric(12, 2) not null,
    price_currency varchar not null,
    is_active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint addon_products_cards_count_check
        check (cards_count > 0),
    constraint addon_products_price_amount_check
        check (price_amount >= 0)
);

create table subscriptions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    plan_id uuid not null
        references plans (id),
    status varchar not null,
    provider varchar,
    provider_subscription_id varchar unique,
    started_at timestamptz not null,
    current_period_start timestamptz,
    current_period_end timestamptz,
    renews_at timestamptz,
    cancels_at timestamptz,
    ended_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint subscriptions_period_order_check
        check (
            current_period_start is null
            or current_period_end is null
            or current_period_end > current_period_start
        )
);

comment on table subscriptions is 'История подписок пользователя. Активная подписка определяется по status.';

create table usage_quotas (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    subscription_id uuid not null
        references subscriptions (id) on delete cascade,
    period_start timestamptz not null,
    period_end timestamptz not null,
    cards_limit integer not null,
    cards_used integer not null default 0,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (user_id, period_start, period_end),
    constraint usage_quotas_period_check
        check (period_end > period_start),
    constraint usage_quotas_cards_limit_check
        check (cards_limit >= 0),
    constraint usage_quotas_cards_used_check
        check (cards_used >= 0 and cards_used <= cards_limit)
);

comment on table usage_quotas is 'Снимок лимитов и использования по периоду подписки.';

create table payments (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    subscription_id uuid
        references subscriptions (id) on delete set null,
    addon_product_id uuid
        references addon_products (id) on delete set null,
    provider varchar not null,
    provider_payment_id varchar unique,
    kind varchar not null,
    status varchar not null,
    amount numeric(12, 2) not null,
    currency varchar not null,
    paid_at timestamptz,
    created_at timestamptz not null default now(),
    constraint payments_kind_check
        check (kind in ('subscription', 'addon')),
    constraint payments_amount_check
        check (amount >= 0)
);

comment on table payments is 'Единая таблица платежей по подпискам и разовым пакетам.';

-- Генерация и файлы.

create table projects (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    title varchar not null,
    marketplace varchar not null default '',
    product_name varchar not null default '',
    product_description text not null default '',
    status varchar not null default 'draft',
    deleted_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint projects_status_check
        check (status in ('draft', 'active', 'archived')),
    constraint projects_title_length_check
        check (char_length(title) between 1 and 200),
    constraint projects_marketplace_length_check
        check (char_length(marketplace) <= 100),
    constraint projects_product_name_length_check
        check (char_length(product_name) <= 255),
    constraint projects_product_description_length_check
        check (char_length(product_description) <= 5000)
);

comment on table projects is 'Рабочая сущность пользователя для генерации карточек.';

create table assets (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    project_id uuid
        references projects (id) on delete set null,
    kind varchar not null,
    storage_key text not null unique,
    mime_type varchar,
    size_bytes bigint,
    original_filename varchar,
    created_at timestamptz not null default now(),
    constraint assets_kind_check
        check (kind in ('source', 'generated', 'export')),
    constraint assets_size_bytes_check
        check (size_bytes is null or size_bytes >= 0)
);

comment on table assets is 'Единая таблица для исходных и результирующих файлов.';

create table generation_jobs (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    project_id uuid not null
        references projects (id) on delete cascade,
    source_asset_id uuid not null
        references assets (id),
    status varchar not null,
    current_step varchar,
    progress_percent integer,
    error_message text,
    archive_asset_id uuid
        references assets (id) on delete set null,
    started_at timestamptz not null,
    finished_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint generation_jobs_progress_percent_check
        check (progress_percent is null or (progress_percent >= 0 and progress_percent <= 100)),
    constraint generation_jobs_finished_at_check
        check (finished_at is null or finished_at >= started_at)
);

comment on table generation_jobs is 'Запуск генерации и его служебный статус.';

create table generated_cards (
    id uuid primary key default gen_random_uuid(),
    generation_job_id uuid not null
        references generation_jobs (id) on delete cascade,
    asset_id uuid not null
        references assets (id),
    card_type_id varchar not null,
    sort_order integer not null,
    created_at timestamptz not null default now(),
    unique (generation_job_id, sort_order),
    constraint generated_cards_sort_order_check
        check (sort_order >= 0)
);

comment on table generated_cards is 'Отдельные карточки, полученные в рамках одного запуска генерации.';

create table export_requests (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    status varchar not null,
    archive_asset_id uuid
        references assets (id) on delete set null,
    requested_at timestamptz not null,
    finished_at timestamptz,
    constraint export_requests_finished_at_check
        check (finished_at is null or finished_at >= requested_at)
);

create table account_deletion_requests (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null
        references users (id) on delete cascade,
    status varchar not null,
    requested_at timestamptz not null,
    scheduled_for timestamptz,
    finished_at timestamptz,
    constraint account_deletion_requests_scheduled_for_check
        check (scheduled_for is null or scheduled_for >= requested_at),
    constraint account_deletion_requests_finished_at_check
        check (finished_at is null or finished_at >= requested_at)
);

-- Блог и SEO-страницы.

create table blog_posts (
    id uuid primary key default gen_random_uuid(),
    slug varchar not null unique,
    title varchar not null,
    excerpt text,
    seo_title varchar,
    seo_description text,
    cover_asset_id uuid
        references assets (id) on delete set null,
    status varchar not null,
    published_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

comment on table blog_posts is 'Статьи блога для SEO-страниц.';

create table blog_post_sections (
    id uuid primary key default gen_random_uuid(),
    blog_post_id uuid not null
        references blog_posts (id) on delete cascade,
    sort_order integer not null,
    kind varchar not null,
    title varchar,
    level integer,
    payload jsonb not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (blog_post_id, sort_order),
    constraint blog_post_sections_sort_order_check
        check (sort_order >= 0),
    constraint blog_post_sections_level_check
        check (level is null or (level >= 1 and level <= 6))
);

comment on table blog_post_sections is 'Универсальные секции статьи. Детали блока лежат в JSON payload.';

create table blog_categories (
    id uuid primary key default gen_random_uuid(),
    slug varchar not null unique,
    label varchar not null,
    created_at timestamptz not null default now()
);

create table blog_post_categories (
    id uuid primary key default gen_random_uuid(),
    blog_post_id uuid not null
        references blog_posts (id) on delete cascade,
    blog_category_id uuid not null
        references blog_categories (id) on delete cascade,
    unique (blog_post_id, blog_category_id)
);

create table blog_tags (
    id uuid primary key default gen_random_uuid(),
    slug varchar not null unique,
    label varchar not null,
    created_at timestamptz not null default now()
);

create table blog_post_tags (
    id uuid primary key default gen_random_uuid(),
    blog_post_id uuid not null
        references blog_posts (id) on delete cascade,
    blog_tag_id uuid not null
        references blog_tags (id) on delete cascade,
    unique (blog_post_id, blog_tag_id)
);

create table blog_post_metrics (
    blog_post_id uuid primary key
        references blog_posts (id) on delete cascade,
    views_count bigint not null default 0,
    reading_time_minutes integer,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint blog_post_metrics_views_count_check
        check (views_count >= 0),
    constraint blog_post_metrics_reading_time_minutes_check
        check (reading_time_minutes is null or reading_time_minutes > 0)
);

-- Индексы.

create index sessions_user_id_idx on sessions (user_id);
create index sessions_expires_at_idx on sessions (expires_at);
create index password_reset_tokens_user_id_idx on password_reset_tokens (user_id);
create index password_reset_tokens_expires_at_idx on password_reset_tokens (expires_at);
create index oauth_accounts_user_id_idx on oauth_accounts (user_id);
create index api_keys_user_id_idx on api_keys (user_id);
create index subscriptions_user_id_idx on subscriptions (user_id);
create index subscriptions_plan_id_idx on subscriptions (plan_id);
create index usage_quotas_user_id_idx on usage_quotas (user_id);
create index usage_quotas_subscription_id_idx on usage_quotas (subscription_id);
create index payments_user_id_idx on payments (user_id);
create index payments_subscription_id_idx on payments (subscription_id);
create index payments_addon_product_id_idx on payments (addon_product_id);
create index projects_user_id_idx on projects (user_id);
create index projects_user_id_updated_at_idx on projects (user_id, updated_at desc);
create index projects_user_id_deleted_at_updated_at_idx on projects (user_id, deleted_at, updated_at desc);
create index assets_user_id_idx on assets (user_id);
create index assets_project_id_idx on assets (project_id);
create index assets_kind_idx on assets (kind);
create index generation_jobs_user_id_idx on generation_jobs (user_id);
create index generation_jobs_project_id_idx on generation_jobs (project_id);
create index generation_jobs_source_asset_id_idx on generation_jobs (source_asset_id);
create index generation_jobs_archive_asset_id_idx on generation_jobs (archive_asset_id);
create index generation_jobs_created_at_idx on generation_jobs (created_at);
create index generated_cards_generation_job_id_idx on generated_cards (generation_job_id);
create index generated_cards_asset_id_idx on generated_cards (asset_id);
create index export_requests_user_id_idx on export_requests (user_id);
create index export_requests_archive_asset_id_idx on export_requests (archive_asset_id);
create index account_deletion_requests_user_id_idx on account_deletion_requests (user_id);
create index blog_posts_status_idx on blog_posts (status);
create index blog_posts_published_at_idx on blog_posts (published_at);
create index blog_posts_cover_asset_id_idx on blog_posts (cover_asset_id);
create index blog_post_sections_blog_post_id_idx on blog_post_sections (blog_post_id);
create index blog_post_categories_blog_category_id_idx on blog_post_categories (blog_category_id);
create index blog_post_tags_blog_tag_id_idx on blog_post_tags (blog_tag_id);
