-- projects хранит рабочие проекты пользователя для генерации карточек.
create table projects (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    title varchar not null,
    marketplace varchar not null default '',
    product_name varchar not null default '',
    product_description text not null default '',
    -- status: draft, active, archived
    status varchar not null default 'draft' check (status in ('draft', 'active', 'archived')),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

comment on table projects is 'Рабочий проект пользователя для генерации карточек маркетплейса.';

create index projects_user_id_idx on projects (user_id);
create index projects_user_id_updated_at_idx on projects (user_id, updated_at desc);
