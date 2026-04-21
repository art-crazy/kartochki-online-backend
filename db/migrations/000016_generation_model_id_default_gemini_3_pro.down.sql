-- Возвращаем прежний SQL default, если миграцию нужно откатить.
alter table generations
    alter column model_id set default 'google/gemini-2.5-flash-image';
