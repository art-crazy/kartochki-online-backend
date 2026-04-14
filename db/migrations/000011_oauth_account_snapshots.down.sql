alter table oauth_accounts
    drop column if exists updated_at,
    drop column if exists avatar_url,
    drop column if exists name;
