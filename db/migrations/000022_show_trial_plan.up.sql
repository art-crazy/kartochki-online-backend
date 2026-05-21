-- Возвращаем тариф Триал в публичный список тарифов и checkout.
update plans
set is_active = true,
    updated_at = now()
where code = 'test';
