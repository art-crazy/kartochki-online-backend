insert into plans (code, name, monthly_price, yearly_monthly_price, cards_per_month, is_popular, is_active)
values ('test', 'Тест', 10, null, 1, false, true)
on conflict (code) do update
set
    name = excluded.name,
    monthly_price = excluded.monthly_price,
    yearly_monthly_price = excluded.yearly_monthly_price,
    cards_per_month = excluded.cards_per_month,
    is_popular = excluded.is_popular,
    is_active = excluded.is_active,
    updated_at = now();
