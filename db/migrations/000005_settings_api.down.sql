alter table sessions
    drop column if exists ip_address,
    drop column if exists user_agent;

drop index if exists api_keys_active_user_idx;
drop table if exists api_keys;
drop table if exists notification_preferences;
drop table if exists user_settings;
