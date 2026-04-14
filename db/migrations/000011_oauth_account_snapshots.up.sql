alter table oauth_accounts
    add column name text,
    add column avatar_url text,
    add column updated_at timestamptz not null default now();

comment on column oauth_accounts.name is 'Снимок имени, которое вернул OAuth-провайдер во время входа.';
comment on column oauth_accounts.avatar_url is 'Снимок аватара, который вернул OAuth-провайдер во время входа.';
comment on column oauth_accounts.updated_at is 'Время последнего обновления снимка OAuth-аккаунта.';
