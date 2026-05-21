-- Скрываем тестовый тариф из публичного списка тарифов и checkout.
update plans
set is_active = false,
    updated_at = now()
where code = 'test';
