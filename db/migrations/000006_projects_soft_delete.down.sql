drop index if exists projects_user_id_deleted_at_updated_at_idx;

alter table projects
    drop constraint if exists projects_title_length_check,
    drop constraint if exists projects_marketplace_length_check,
    drop constraint if exists projects_product_name_length_check,
    drop constraint if exists projects_product_description_length_check;

alter table projects
    drop column if exists deleted_at;
