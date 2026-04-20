alter table user_settings
    add column avatar_asset_id uuid references assets (id) on delete set null;

comment on column user_settings.avatar_asset_id is 'Пользовательский аватар, загруженный через страницу настроек.';
