-- name: CreateAsset :one
-- Создаёт запись о файле после того, как storage уже сохранил его по storage_key.
insert into assets (id, user_id, kind, storage_key, original_filename, mime_type, size_bytes)
values (@id, @user_id, @kind, @storage_key, @original_filename, @mime_type, @size_bytes)
returning *;

-- name: GetAssetByID :one
select * from assets
where id = @id;

-- name: DeleteAssetByID :execrows
delete from assets
where id = @id;

-- name: GetUserAssetByID :one
-- Позволяет клиенту использовать только свои загруженные исходники.
select * from assets
where id = @id
  and user_id = @user_id;

-- name: CreateGeneration :one
insert into generations (
    id,
    user_id,
    project_id,
    source_asset_id,
    marketplace_id,
    style_id,
    card_count,
    model_id,
    status,
    current_step,
    progress_percent
)
values (
    @id,
    @user_id,
    @project_id,
    @source_asset_id,
    @marketplace_id,
    @style_id,
    @card_count,
    @model_id,
    'queued',
    'queued',
    0
)
returning *;

-- name: AddGenerationCardType :exec
insert into generation_card_types (generation_id, position, card_type_id)
values (@generation_id, @position, @card_type_id);

-- name: GetGenerationByID :one
select * from generations
where id = @id;

-- name: GetUserGenerationByID :one
select
    g.*,
    coalesce(archive.storage_key, '') as archive_storage_key
from generations g
left join assets archive on archive.id = g.archive_asset_id
where g.id = @id
  and g.user_id = @user_id;

-- name: ListGenerationCardTypes :many
select generation_id, position, card_type_id
from generation_card_types
where generation_id = @generation_id
order by position asc;

-- name: MarkGenerationProcessing :execrows
-- Первый worker-переход фиксирует старт обработки и защищает от повторного запуска завершённой задачи.
update generations
set status = 'processing',
    current_step = @current_step,
    progress_percent = @progress_percent,
    error_message = '',
    started_at = coalesce(started_at, now()),
    updated_at = now()
where id = @id
  and status in ('queued', 'processing');

-- name: UpdateGenerationProgress :execrows
update generations
set current_step = @current_step,
    progress_percent = @progress_percent,
    updated_at = now()
where id = @id
  and status = 'processing';

-- name: MarkGenerationFailed :exec
update generations
set status = 'failed',
    current_step = 'failed',
    error_message = @error_message,
    updated_at = now(),
    finished_at = now()
where id = @id;

-- name: MarkGenerationCompleted :exec
update generations
set status = 'completed',
    current_step = 'completed',
    progress_percent = 100,
    archive_asset_id = @archive_asset_id,
    updated_at = now(),
    finished_at = now()
where id = @id;

-- name: CreateGeneratedCard :one
insert into generated_cards (generation_id, asset_id, card_type_id, position)
values (@generation_id, @asset_id, @card_type_id, @position)
returning *;

-- name: ListGeneratedCardsByGenerationID :many
select
    gc.id,
    gc.card_type_id,
    gc.asset_id,
    a.storage_key
from generated_cards gc
join assets a on a.id = gc.asset_id
where gc.generation_id = @generation_id
order by gc.position asc;

-- name: DeleteGeneratedCardsByGenerationID :exec
delete from generated_cards
where generation_id = @generation_id;

-- name: ClearGenerationArchiveAsset :exec
update generations
set archive_asset_id = null,
    updated_at = now()
where id = @id;

-- name: ActivateProjectByID :execrows
-- После успешной генерации проект становится активным и начинает выглядеть как рабочий.
update projects
set status = 'active',
    updated_at = now()
where id = @id
  and deleted_at is null;

-- name: CreateGenerationProductContext :one
-- Сохраняет контекст товара, переданный пользователем при запуске генерации.
-- Вызывается в той же транзакции, что и создание generation, чтобы откат был атомарным.
insert into generation_product_context (generation_id, name, category, brand, description, benefits, characteristics)
values (@generation_id, @name, @category, @brand, @description, @benefits, @characteristics)
returning *;

-- name: GetGenerationProductContextByGenerationID :one
-- Возвращает контекст товара для generation перед запуском prompt builder в worker.
select * from generation_product_context
where generation_id = @generation_id;
