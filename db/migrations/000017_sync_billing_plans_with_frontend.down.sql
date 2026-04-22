delete from plans
where code = 'corporate';

update plans
set
    code = 'pro',
    name = 'Профи',
    monthly_price = 1490,
    yearly_monthly_price = 990,
    cards_per_month = 500,
    is_popular = true,
    updated_at = now()
where code = 'business';

update plans
set
    code = 'business',
    name = 'Бизнес',
    monthly_price = 4990,
    yearly_monthly_price = 3490,
    cards_per_month = 2500,
    is_popular = false,
    updated_at = now()
where code = 'agency';

update plans
set
    name = 'Старт',
    monthly_price = 0,
    yearly_monthly_price = null,
    cards_per_month = 30,
    is_popular = false,
    updated_at = now()
where code = 'free';
