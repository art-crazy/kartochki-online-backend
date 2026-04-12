create table plans (
    id uuid primary key default gen_random_uuid(),
    code varchar not null unique,
    name varchar not null,
    monthly_price integer not null check (monthly_price >= 0),
    yearly_monthly_price integer check (yearly_monthly_price is null or yearly_monthly_price >= 0),
    cards_per_month integer not null check (cards_per_month >= 0),
    is_popular boolean not null default false,
    is_active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

comment on table plans is 'Тарифные планы, которые backend показывает на странице billing.';

create table addon_products (
    id uuid primary key default gen_random_uuid(),
    code varchar not null unique,
    title varchar not null,
    description text not null default '',
    price integer not null check (price >= 0),
    cards_count integer not null check (cards_count > 0),
    is_active boolean not null default true,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

comment on table addon_products is 'Разовые пакеты карточек для покупки сверх месячного лимита.';

create table subscriptions (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    plan_id uuid not null references plans (id),
    status varchar not null check (status in ('active', 'scheduled_cancel', 'canceled')),
    provider varchar not null default 'manual',
    provider_subscription_id varchar unique,
    has_payment_method boolean not null default false,
    started_at timestamptz not null,
    current_period_start timestamptz not null,
    current_period_end timestamptz not null,
    renews_at timestamptz,
    cancels_at timestamptz,
    ended_at timestamptz,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    constraint subscriptions_period_check check (current_period_end > current_period_start)
);

comment on table subscriptions is 'Текущее и прошлые состояния подписки пользователя по тарифу.';

create unique index subscriptions_active_user_idx
    on subscriptions (user_id)
    where status in ('active', 'scheduled_cancel');

create table usage_quotas (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    subscription_id uuid not null references subscriptions (id) on delete cascade,
    period_start timestamptz not null,
    period_end timestamptz not null,
    cards_limit integer not null check (cards_limit >= 0),
    cards_used integer not null default 0 check (cards_used >= 0),
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    unique (subscription_id, period_start, period_end),
    constraint usage_quotas_period_check check (period_end > period_start)
);

comment on table usage_quotas is 'Снимок лимита на период подписки. cards_used пока хранится для будущей синхронизации.';

create table payments (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references users (id) on delete cascade,
    subscription_id uuid references subscriptions (id) on delete set null,
    addon_product_id uuid references addon_products (id) on delete set null,
    provider varchar not null,
    provider_payment_id varchar unique,
    kind varchar not null check (kind in ('subscription', 'addon')),
    status varchar not null check (status in ('pending', 'paid', 'canceled')),
    amount integer not null check (amount >= 0),
    currency varchar not null,
    checkout_url text,
    paid_at timestamptz,
    created_at timestamptz not null default now()
);

comment on table payments is 'Платежи и checkout-сессии. Реальная интеграция провайдера сможет безопасно использовать эту таблицу позже.';

create index plans_active_idx on plans (is_active);
create index addon_products_active_idx on addon_products (is_active);
create index subscriptions_user_id_created_at_idx on subscriptions (user_id, created_at desc);
create index usage_quotas_user_id_period_idx on usage_quotas (user_id, period_start desc);
create index payments_user_id_created_at_idx on payments (user_id, created_at desc);

insert into plans (code, name, monthly_price, yearly_monthly_price, cards_per_month, is_popular)
values
    ('free', 'Старт', 0, null, 30, false),
    ('pro', 'Профи', 1490, 990, 500, true),
    ('business', 'Бизнес', 4990, 3490, 2500, false);

insert into addon_products (code, title, description, price, cards_count)
values
    ('cards_100', '100 карточек', 'Подходит, когда месячного лимита не хватает до следующего периода.', 490, 100),
    ('cards_500', '500 карточек', 'Разовый пакет для активных запусков и сезонных пиков.', 1990, 500);
