drop index if exists users_email_unique_not_null_idx;

-- При откате нужно убрать Telegram-пользователей без email,
-- иначе PostgreSQL не даст вернуть обязательность поля.
delete from users
where email is null;

alter table users
    alter column email set not null;

alter table users
    add constraint users_email_key unique (email);
