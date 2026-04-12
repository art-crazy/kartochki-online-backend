-- name: CreateProject :one
-- Создаёт новый проект пользователя со статусом draft.
insert into projects (user_id, title, marketplace, product_name, product_description, status)
values (@user_id, @title, @marketplace, @product_name, @product_description, 'draft')
returning *;

-- name: GetProjectByID :one
-- Возвращает только активный проект его владельца.
-- Мягко удалённые проекты снаружи считаются несуществующими.
select * from projects
where id = @id
  and user_id = @user_id
  and deleted_at is null;

-- name: ListUserProjects :many
-- Возвращает только активные проекты пользователя,
-- отсортированные по дате последнего обновления.
select * from projects
where user_id = @user_id
  and deleted_at is null
order by updated_at desc;

-- name: UpdateProject :one
-- Обновляет поля только у активного проекта.
-- user_id в WHERE гарантирует, что чужой проект нельзя изменить.
update projects
set title               = @title,
    marketplace         = @marketplace,
    product_name        = @product_name,
    product_description = @product_description,
    updated_at          = now()
where id = @id
  and user_id = @user_id
  and deleted_at is null
returning *;

-- name: SoftDeleteProject :execrows
-- Мягко удаляет проект: он исчезает из пользовательских списков,
-- но остаётся в БД для истории, связей и будущих фоновых задач.
update projects
set status = 'archived',
    deleted_at = now(),
    updated_at = now()
where id = @id
  and user_id = @user_id
  and deleted_at is null;
