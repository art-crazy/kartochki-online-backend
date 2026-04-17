create table pending_registrations (
    id uuid primary key default gen_random_uuid(),
    email varchar not null unique,
    name varchar not null default '',
    password_hash varchar not null,
    verification_code_hash varchar not null,
    verification_expires_at timestamptz not null,
    resend_available_at timestamptz not null,
    attempt_count integer not null default 0,
    resend_count integer not null default 1,
    status varchar not null default 'pending',
    last_email_sent_at timestamptz not null default now(),
    created_ip_address varchar not null default '',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now(),
    completed_at timestamptz,
    constraint pending_registrations_attempt_count_check check (attempt_count >= 0),
    constraint pending_registrations_resend_count_check check (resend_count >= 0),
    constraint pending_registrations_status_check check (status in ('pending', 'verified', 'expired', 'blocked'))
);

comment on table pending_registrations is 'Неподтверждённые регистрации по email/паролю до ввода одноразового кода.';
comment on column pending_registrations.verification_code_hash is 'Хэш одноразового кода из письма. Сырой код в базе не хранится.';
comment on column pending_registrations.resend_available_at is 'Момент, раньше которого повторная отправка кода запрещена.';
comment on column pending_registrations.attempt_count is 'Количество неверных попыток ввода кода для текущего flow.';
comment on column pending_registrations.resend_count is 'Количество уже отправленных кодов в рамках одного flow.';
comment on column pending_registrations.status is 'Состояние flow: pending, verified, expired или blocked.';
comment on column pending_registrations.completed_at is 'Когда flow был завершён подтверждением или финально закрыт.';

create index pending_registrations_created_ip_address_idx on pending_registrations (created_ip_address, created_at desc);
create index pending_registrations_created_at_idx on pending_registrations (created_at desc);
