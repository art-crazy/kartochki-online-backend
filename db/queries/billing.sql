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
