update plans
set
    code = 'agency',
    name = 'Агентство',
    monthly_price = 4990,
    yearly_monthly_price = 3992,
    cards_per_month = 250,
    is_popular = false,
    updated_at = now()
where code = 'business';

update plans
set
    code = 'business',
    name = 'Бизнес',
    monthly_price = 1490,
    yearly_monthly_price = 1192,
    cards_per_month = 75,
    is_popular = true,
    updated_at = now()
where code = 'pro';

update plans
set
    name = 'Старт',
    monthly_price = 0,
    yearly_monthly_price = null,
    cards_per_month = 5,
    is_popular = false,
    updated_at = now()
where code = 'free';

insert into plans (code, name, monthly_price, yearly_monthly_price, cards_per_month, is_popular, is_active)
values ('corporate', 'Корпоративный', 14990, 11992, 750, false, true)
on conflict (code) do update
set
    name = excluded.name,
    monthly_price = excluded.monthly_price,
    yearly_monthly_price = excluded.yearly_monthly_price,
    cards_per_month = excluded.cards_per_month,
    is_popular = excluded.is_popular,
    is_active = excluded.is_active,
    updated_at = now();
