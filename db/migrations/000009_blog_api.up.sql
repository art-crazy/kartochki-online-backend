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

create index blog_posts_status_idx on blog_posts (status);
create index blog_posts_published_at_idx on blog_posts (published_at);
create index blog_posts_cover_asset_id_idx on blog_posts (cover_asset_id);
create index blog_post_sections_blog_post_id_idx on blog_post_sections (blog_post_id);
create index blog_post_categories_blog_category_id_idx on blog_post_categories (blog_category_id);
create index blog_post_tags_blog_tag_id_idx on blog_post_tags (blog_tag_id);
