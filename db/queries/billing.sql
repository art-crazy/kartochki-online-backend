-- name: ListActiveBillingPlans :many
select
    id,
    code,
    name,
    monthly_price,
    yearly_monthly_price,
    cards_per_month,
    is_popular,
    is_active,
    created_at,
    updated_at
from plans
where is_active = true
order by monthly_price asc, created_at asc;

-- name: GetBillingPlanByCode :one
select
    id,
    code,
    name,
    monthly_price,
    yearly_monthly_price,
    cards_per_month,
    is_popular,
    is_active,
    created_at,
    updated_at
from plans
where code = @code
  and is_active = true
limit 1;

-- name: ListActiveAddonProducts :many
select
    id,
    code,
    title,
    description,
    price,
    cards_count,
    is_active,
    created_at,
    updated_at
from addon_products
where is_active = true
order by cards_count asc, created_at asc;

-- name: GetAddonProductByCode :one
select
    id,
    code,
    title,
    description,
    price,
    cards_count,
    is_active,
    created_at,
    updated_at
from addon_products
where code = @code
  and is_active = true
limit 1;

-- name: GetCurrentSubscriptionByUserID :one
select
    s.id,
    s.user_id,
    s.plan_id,
    s.status,
    s.provider,
    s.provider_subscription_id,
    s.has_payment_method,
    s.started_at,
    s.current_period_start,
    s.current_period_end,
    s.renews_at,
    s.cancels_at,
    s.ended_at,
    s.created_at,
    s.updated_at,
    p.code as plan_code,
    p.name as plan_name,
    p.cards_per_month
from subscriptions s
join plans p on p.id = s.plan_id
where s.user_id = @user_id
  and s.status in ('active', 'scheduled_cancel')
order by s.created_at desc
limit 1;

-- name: CreateSubscription :one
insert into subscriptions (
    user_id,
    plan_id,
    status,
    provider,
    has_payment_method,
    started_at,
    current_period_start,
    current_period_end,
    renews_at,
    cancels_at
) values (
    @user_id,
    @plan_id,
    @status,
    @provider,
    @has_payment_method,
    @started_at,
    @current_period_start,
    @current_period_end,
    @renews_at,
    @cancels_at
)
returning *;

-- name: MarkSubscriptionScheduledCancel :execrows
update subscriptions
set status = 'scheduled_cancel',
    cancels_at = @cancels_at,
    updated_at = now()
where id = @id
  and user_id = @user_id
  and status = 'active';

-- name: GetCurrentUsageQuotaBySubscriptionID :one
select
    id,
    user_id,
    subscription_id,
    period_start,
    period_end,
    cards_limit,
    cards_used,
    created_at,
    updated_at
from usage_quotas
where subscription_id = @subscription_id
  and period_start <= @now_at
  and period_end > @now_at
order by period_start desc
limit 1;

-- name: CreateUsageQuota :one
insert into usage_quotas (
    user_id,
    subscription_id,
    period_start,
    period_end,
    cards_limit,
    cards_used
) values (
    @user_id,
    @subscription_id,
    @period_start,
    @period_end,
    @cards_limit,
    @cards_used
)
returning *;

-- name: CountGeneratedCardsForUserInPeriod :one
-- Считаем готовые карточки по факту из generated_cards, чтобы billing-экран не зависел от фоновой синхронизации usage_quotas.
select count(*)
from generated_cards gc
join generations g on g.id = gc.generation_id
where g.user_id = @user_id
  and g.status = 'completed'
  and gc.created_at >= @period_start
  and gc.created_at < @period_end;

-- name: SumReservedGenerationCardsForUserInPeriod :one
-- Для проверки лимита учитываем уже созданные generation, которые ещё не завершились ошибкой.
-- Так пользователь не сможет переполнить квоту пачкой параллельных запусков.
select coalesce(sum(card_count), 0)::bigint
from generations
where user_id = @user_id
  and status in ('queued', 'processing', 'completed')
  and created_at >= @period_start
  and created_at < @period_end;

-- name: GetPaymentByProviderID :one
-- Ищем платёж по внешнему ID провайдера для идемпотентной обработки webhook.
select
    id,
    user_id,
    subscription_id,
    addon_product_id,
    provider,
    provider_payment_id,
    kind,
    status,
    amount,
    currency,
    checkout_url,
    paid_at,
    created_at
from payments
where provider_payment_id = @provider_payment_id
limit 1;

-- name: ListSubscriptionsDueForRenewal :many
-- Находим активные подписки, которым пора создать рекуррентный платёж.
-- pending-платёж по той же подписке означает, что попытка уже создана и ждёт webhook.
select
    s.id,
    s.user_id,
    s.plan_id,
    s.provider,
    s.provider_subscription_id,
    s.current_period_start,
    s.current_period_end,
    s.renews_at,
    p.code as plan_code,
    p.monthly_price,
    p.yearly_monthly_price,
    p.cards_per_month
from subscriptions s
join plans p on p.id = s.plan_id
where s.status = 'active'
  and s.has_payment_method = true
  and s.provider = 'yookassa'
  and s.provider_subscription_id is not null
  and s.renews_at <= @now_at
  and not exists (
      select 1
      from payments pay
      where pay.subscription_id = s.id
        and pay.kind = 'subscription'
        and pay.status = 'pending'
  )
order by s.renews_at asc
limit sqlc.arg(batch_limit);

-- name: CreatePayment :one
-- Создаём запись платежа после получения checkout от провайдера.
insert into payments (
    user_id,
    subscription_id,
    addon_product_id,
    provider,
    provider_payment_id,
    kind,
    status,
    amount,
    currency,
    checkout_url
) values (
    @user_id,
    @subscription_id,
    @addon_product_id,
    @provider,
    @provider_payment_id,
    @kind,
    @status,
    @amount,
    @currency,
    @checkout_url
)
returning *;

-- name: MarkPaymentPaid :execrows
-- Фиксируем успешное завершение платежа после webhook payment.succeeded.
update payments
set status  = 'paid',
    paid_at = @paid_at
where provider_payment_id = @provider_payment_id
  and status = 'pending';

-- name: MarkPaymentCanceled :execrows
-- Помечаем платёж отменённым после webhook payment.canceled.
update payments
set status = 'canceled'
where provider_payment_id = @provider_payment_id
  and status = 'pending';

-- name: UpsertActiveSubscription :one
-- Создаёт или обновляет активную подписку после успешного платежа.
-- ON CONFLICT по user_id гарантирует, что у пользователя всегда одна активная запись.
insert into subscriptions (
    user_id,
    plan_id,
    status,
    provider,
    provider_subscription_id,
    has_payment_method,
    started_at,
    current_period_start,
    current_period_end,
    renews_at,
    cancels_at
) values (
    @user_id,
    @plan_id,
    'active',
    @provider,
    @provider_subscription_id,
    @has_payment_method,
    @started_at,
    @current_period_start,
    @current_period_end,
    @renews_at,
    null
)
on conflict (user_id) where status in ('active', 'scheduled_cancel')
do update set
    plan_id                  = excluded.plan_id,
    status                   = 'active',
    provider                 = excluded.provider,
    provider_subscription_id = excluded.provider_subscription_id,
    has_payment_method       = excluded.has_payment_method,
    current_period_start     = excluded.current_period_start,
    current_period_end       = excluded.current_period_end,
    renews_at                = excluded.renews_at,
    cancels_at               = null,
    updated_at               = now()
returning *;

-- name: UpsertUsageQuotaForSubscription :one
-- Создаёт или сбрасывает usage_quota при начале нового billing-периода.
insert into usage_quotas (
    user_id,
    subscription_id,
    period_start,
    period_end,
    cards_limit,
    cards_used
) values (
    @user_id,
    @subscription_id,
    @period_start,
    @period_end,
    @cards_limit,
    0
)
on conflict (subscription_id, period_start, period_end)
do update set
    cards_limit = excluded.cards_limit,
    updated_at  = now()
returning *;

-- name: AddAddonCardsToQuota :execrows
-- Увеличивает лимит карточек в текущей квоте пользователя после покупки addon.
-- Подзапрос нужен, потому что PostgreSQL не поддерживает ORDER BY/LIMIT в UPDATE напрямую.
update usage_quotas uq
set cards_limit = uq.cards_limit + sqlc.arg(extra_cards),
    updated_at  = now()
where uq.id = (
    select sub.id
    from usage_quotas sub
    where sub.user_id = sqlc.arg(user_id)
      and sub.period_start <= sqlc.arg(now_at)
      and sub.period_end > sqlc.arg(now_at)
    order by sub.period_start desc
    limit 1
);

-- name: GetSubscriptionByID :one
select
    id,
    user_id,
    plan_id,
    status,
    provider,
    provider_subscription_id,
    has_payment_method,
    started_at,
    current_period_start,
    current_period_end,
    renews_at,
    cancels_at,
    ended_at,
    created_at,
    updated_at
from subscriptions
where id = @id;
