drop index if exists oauth_accounts_user_id_idx;
drop index if exists sessions_expires_at_idx;
drop index if exists sessions_user_id_idx;

drop table if exists oauth_accounts;
drop table if exists sessions;
drop table if exists users;
