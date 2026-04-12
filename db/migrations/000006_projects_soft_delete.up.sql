-- deleted_at позволяет "удалять" проект без физической потери строки.
-- Это нужно, чтобы позже можно было безопасно связать проект с генерациями,
-- файлами и аудитом, не рискуя каскадно потерять историю пользователя.
alter table projects
    add column deleted_at timestamptz;

comment on column projects.deleted_at is 'Момент мягкого удаления проекта. NULL означает, что проект активен.';

-- Дублируем лимиты полей на уровне БД, чтобы данные не испортились даже
-- если в одном из entrypoint появится пропущенная или старая валидация.
alter table projects
    add constraint projects_title_length_check
        check (char_length(title) between 1 and 200),
    add constraint projects_marketplace_length_check
        check (char_length(marketplace) <= 100),
    add constraint projects_product_name_length_check
        check (char_length(product_name) <= 255),
    add constraint projects_product_description_length_check
        check (char_length(product_description) <= 5000);

-- Индекс ускоряет выборку живых проектов пользователя на страницах списка и дашборда.
create index projects_user_id_deleted_at_updated_at_idx on projects (user_id, deleted_at, updated_at desc);
