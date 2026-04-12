-- Telegram Login Widget не гарантирует email.
-- Поэтому email остаётся уникальным, но больше не обязателен для всех аккаунтов.
alter table users
    drop constraint if exists users_email_key;

alter table users
    alter column email drop not null;

create unique index if not exists users_email_unique_not_null_idx
    on users (email)
    where email is not null;
