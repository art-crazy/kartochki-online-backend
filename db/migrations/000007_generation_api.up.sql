-- assets хранит загруженные исходники и файлы, которые backend создаёт в ходе генерации.
create table assets (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    -- kind помогает не смешивать исходники пользователя, готовые карточки и zip-архивы.
    kind varchar not null check (kind in ('source_image', 'generated_card', 'archive')),
    storage_key varchar not null unique,
    original_filename varchar not null default '',
    mime_type varchar not null default 'application/octet-stream',
    size_bytes bigint not null default 0,
    created_at timestamptz not null default now()
);

comment on table assets is 'Файлы пользователя и артефакты генерации.';

create index assets_user_id_created_at_idx on assets (user_id, created_at desc);

-- generations хранит одну попытку генерации карточек и её текущее состояние для polling API.
create table generations (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    project_id uuid not null references projects (id) on delete cascade,
    source_asset_id uuid not null references assets (id) on delete restrict,
    marketplace_id varchar not null,
    style_id varchar not null,
    card_count integer not null check (card_count > 0),
    -- status: queued, processing, completed, failed
    status varchar not null default 'queued' check (status in ('queued', 'processing', 'completed', 'failed')),
    current_step varchar not null default 'queued',
    progress_percent integer not null default 0 check (progress_percent between 0 and 100),
    error_message text not null default '',
    archive_asset_id uuid references assets (id) on delete set null,
    started_at timestamptz,
    finished_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

comment on table generations is 'История запусков генерации карточек и прогресс фоновой задачи.';

create index generations_user_id_created_at_idx on generations (user_id, created_at desc);
create index generations_project_id_created_at_idx on generations (project_id, created_at desc);

-- generation_card_types хранит состав генерации в явном виде, чтобы worker не зависел от массивов в SQL.
create table generation_card_types (
    generation_id uuid not null references generations (id) on delete cascade,
    position integer not null check (position >= 0),
    card_type_id varchar not null,
    primary key (generation_id, position)
);

comment on table generation_card_types is 'Выбранные типы карточек внутри конкретного запуска генерации.';

-- generated_cards хранит итоговые карточки, которые backend связал с generation и asset.
create table generated_cards (
    id uuid primary key default gen_random_uuid(),
    generation_id uuid not null references generations (id) on delete cascade,
    asset_id uuid not null references assets (id) on delete cascade,
    card_type_id varchar not null,
    position integer not null check (position >= 0),
    created_at timestamptz not null default now()
);

comment on table generated_cards is 'Готовые карточки, полученные после завершения генерации.';

create unique index generated_cards_generation_position_idx on generated_cards (generation_id, position);
