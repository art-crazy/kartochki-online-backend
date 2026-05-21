-- Возвращаем тариф Триал в публичный список, если миграцию откатывают.
update plans
set is_active = true,
    updated_at = now()
where code = 'test';
