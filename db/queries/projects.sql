-- name: CreateProject :one
-- Создаёт новый проект со статусом draft.
insert into projects (user_id, title, marketplace, product_name, product_description, status)
values (@user_id, @title, @marketplace, @product_name, @product_description, 'draft')
returning *;

-- name: GetProjectByID :one
-- Возвращает проект только для его владельца.
select * from projects
where id = @id
  and user_id = @user_id;

-- name: ListUserProjects :many
-- Возвращает все проекты пользователя, отсортированные по дате обновления.
select * from projects
where user_id = @user_id
order by updated_at desc;

-- name: UpdateProject :one
-- Обновляет поля проекта. user_id в WHERE гарантирует, что чужой проект не изменить.
update projects
set title               = @title,
    marketplace         = @marketplace,
    product_name        = @product_name,
    product_description = @product_description,
    updated_at          = now()
where id = @id and user_id = @user_id
returning *;

-- name: DeleteProject :execrows
-- Удаляет проект. Возвращает количество удалённых строк для проверки владения.
delete from projects where id = @id and user_id = @user_id;
