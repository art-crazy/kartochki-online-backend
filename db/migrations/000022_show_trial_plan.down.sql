-- При откате снова скрываем тариф Триал из публичного списка.
update plans
set is_active = false,
    updated_at = now()
where code = 'test';
