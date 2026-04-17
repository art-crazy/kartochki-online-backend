-- name: CountRecentPendingRegistrationsByEmail :one
-- Считаем недавние старты регистрации по email, чтобы ограничить перезапуск flow.
select count(*)
from pending_registrations
where email = $1
  and created_at >= $2;

-- name: CountRecentPendingRegistrationsByIP :one
-- Считаем недавние старты регистрации по IP, чтобы не дать спамить новыми flow.
select count(*)
from pending_registrations
where created_ip_address = $1
  and created_at >= $2;

-- name: GetPendingRegistrationByEmail :one
select
    id,
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address,
    created_at,
    updated_at,
    completed_at
from pending_registrations
where email = $1
limit 1;

-- name: GetPendingRegistrationByEmailForUpdate :one
-- Блокируем flow по email, чтобы параллельные register-запросы не потеряли обновление имени или пароля.
select
    id,
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address,
    created_at,
    updated_at,
    completed_at
from pending_registrations
where email = $1
limit 1
for update;

-- name: GetPendingRegistrationForUpdate :one
-- FOR UPDATE нужен, чтобы verify и resend не меняли один flow одновременно.
select
    id,
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address,
    created_at,
    updated_at,
    completed_at
from pending_registrations
where id = $1
limit 1
for update;

-- name: CreatePendingRegistration :one
insert into pending_registrations (
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address
) values (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    0,
    1,
    'pending',
    $7,
    $8
)
returning
    id,
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address,
    created_at,
    updated_at,
    completed_at;

-- name: UpdatePendingRegistrationProfile :one
-- Обновляем имя и пароль без выпуска нового кода, когда пользователь повторно отправил register до истечения flow.
update pending_registrations
set name = $2,
    password_hash = $3,
    updated_at = now()
where id = $1
returning
    id,
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address,
    created_at,
    updated_at,
    completed_at;

-- name: RefreshPendingRegistrationCode :one
-- Обновляем код и таймеры, не создавая новый flow для того же email.
update pending_registrations
set name = $2,
    password_hash = $3,
    verification_code_hash = $4,
    verification_expires_at = $5,
    resend_available_at = $6,
    resend_count = 1,
    attempt_count = 0,
    status = 'pending',
    last_email_sent_at = $7,
    created_ip_address = $8,
    updated_at = now(),
    completed_at = null
where id = $1
returning
    id,
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address,
    created_at,
    updated_at,
    completed_at;

-- name: ResendPendingRegistrationCode :one
-- Повторная отправка всегда заменяет старый код новым и сбрасывает счётчик попыток.
update pending_registrations
set verification_code_hash = $2,
    verification_expires_at = $3,
    resend_available_at = $4,
    resend_count = $5,
    attempt_count = 0,
    last_email_sent_at = $6,
    status = 'pending',
    updated_at = now()
where id = $1
returning
    id,
    email,
    name,
    password_hash,
    verification_code_hash,
    verification_expires_at,
    resend_available_at,
    attempt_count,
    resend_count,
    status,
    last_email_sent_at,
    created_ip_address,
    created_at,
    updated_at,
    completed_at;

-- name: IncrementPendingRegistrationAttempt :one
-- Возвращаем новое значение, чтобы сервис сразу понял, пора ли блокировать flow.
update pending_registrations
set attempt_count = attempt_count + 1,
    updated_at = now()
where id = $1
returning attempt_count;

-- name: CompletePendingRegistration :execrows
update pending_registrations
set status = 'verified',
    completed_at = now(),
    updated_at = now()
where id = $1
  and status = 'pending';

-- name: ExpirePendingRegistration :execrows
update pending_registrations
set status = 'expired',
    completed_at = now(),
    updated_at = now()
where id = $1
  and status = 'pending';

-- name: BlockPendingRegistration :execrows
update pending_registrations
set status = 'blocked',
    completed_at = now(),
    updated_at = now()
where id = $1
  and status = 'pending';
