-- name: GetFeaturedBlogPost :one
-- Возвращает последнюю опубликованную статью для hero-блока списка `/blog`.
select
    p.id,
    p.slug,
    p.title,
    coalesce(p.excerpt, '') as excerpt,
    p.published_at,
    coalesce(m.reading_time_minutes, 1) as reading_time_minutes
from blog_posts p
left join blog_post_metrics m on m.blog_post_id = p.id
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
order by p.published_at desc, p.created_at desc
limit 1;

-- name: CountPublishedBlogPosts :one
-- Считает статьи для серверной пагинации публичного списка.
select count(*)::bigint
from blog_posts p
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now();

-- name: CountPublishedBlogPostsExcludingPost :one
-- Считает статьи списка без featured-материала, чтобы не дублировать его на первой странице.
select count(*)::bigint
from blog_posts p
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
  and p.id <> @blog_post_id;

-- name: ListPublishedBlogPosts :many
-- Возвращает страницу опубликованных статей без дополнительных фильтров.
select
    p.id,
    p.slug,
    p.title,
    coalesce(p.excerpt, '') as excerpt,
    coalesce(c.label, '') as category_label,
    p.published_at,
    coalesce(m.reading_time_minutes, 1) as reading_time_minutes
from blog_posts p
left join blog_post_metrics m on m.blog_post_id = p.id
left join lateral (
    select bc.label
    from blog_post_categories pc
    join blog_categories bc on bc.id = pc.blog_category_id
    where pc.blog_post_id = p.id
    order by bc.label asc
    limit 1
) c on true
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
order by p.published_at desc, p.created_at desc
limit @limit_rows
offset @offset_rows;

-- name: ListPublishedBlogPostsExcludingPost :many
-- Возвращает страницу опубликованных статей без featured-материала, который уже показан в hero-блоке.
select
    p.id,
    p.slug,
    p.title,
    coalesce(p.excerpt, '') as excerpt,
    coalesce(c.label, '') as category_label,
    p.published_at,
    coalesce(m.reading_time_minutes, 1) as reading_time_minutes
from blog_posts p
left join blog_post_metrics m on m.blog_post_id = p.id
left join lateral (
    select bc.label
    from blog_post_categories pc
    join blog_categories bc on bc.id = pc.blog_category_id
    where pc.blog_post_id = p.id
    order by bc.label asc
    limit 1
) c on true
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
  and p.id <> @blog_post_id
order by p.published_at desc, p.created_at desc
limit @limit_rows
offset @offset_rows;

-- name: ListBlogCategories :many
-- Возвращает категории с количеством опубликованных статей в каждой из них.
select
    c.slug,
    c.label,
    count(*)::bigint as posts_count
from blog_categories c
join blog_post_categories pc on pc.blog_category_id = c.id
join blog_posts p on p.id = pc.blog_post_id
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
group by c.id, c.slug, c.label
order by posts_count desc, c.label asc;

-- name: ListPopularBlogPosts :many
-- Возвращает популярные статьи для бокового блока.
select
    p.slug,
    p.title
from blog_posts p
left join blog_post_metrics m on m.blog_post_id = p.id
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
order by coalesce(m.views_count, 0) desc, p.published_at desc, p.created_at desc
limit 5;

-- name: ListBlogTags :many
-- Возвращает теги, которые используются в опубликованных статьях.
select distinct
    t.slug,
    t.label
from blog_tags t
join blog_post_tags pt on pt.blog_tag_id = t.id
join blog_posts p on p.id = pt.blog_post_id
where p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
order by t.label asc;

-- name: GetPublishedBlogPostBySlug :one
-- Возвращает опубликованную статью по slug вместе с SEO-данными и метриками.
select
    p.id,
    p.slug,
    p.title,
    coalesce(p.seo_description, '') as seo_description,
    coalesce(p.excerpt, '') as excerpt,
    p.published_at,
    p.updated_at,
    coalesce(m.reading_time_minutes, 1) as reading_time_minutes,
    coalesce(m.views_count, 0)::bigint as views_count
from blog_posts p
left join blog_post_metrics m on m.blog_post_id = p.id
where p.slug = @slug
  and p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now();

-- name: ListBlogPostSections :many
-- Возвращает секции статьи в порядке, в котором их нужно рендерить на странице.
select
    id,
    blog_post_id,
    sort_order,
    kind,
    coalesce(title, '') as title,
    coalesce(level, 2) as level,
    payload,
    created_at,
    updated_at
from blog_post_sections
where blog_post_id = @blog_post_id
order by sort_order asc, created_at asc;

-- name: ListBlogPostTags :many
-- Возвращает теги статьи для шапки и SEO-блоков.
select
    t.slug,
    t.label
from blog_tags t
join blog_post_tags pt on pt.blog_tag_id = t.id
where pt.blog_post_id = @blog_post_id
order by t.label asc;

-- name: ListRelatedBlogPostsByPostID :many
-- Возвращает похожие статьи, сначала по общим категориям, затем по дате публикации.
select
    p.slug,
    p.title
from blog_posts p
left join blog_post_categories pc
    on pc.blog_post_id = p.id
   and pc.blog_category_id in (
        select blog_category_id
        from blog_post_categories
        where blog_post_categories.blog_post_id = @blog_post_id
   )
where p.id <> @blog_post_id
  and p.status = 'published'
  and p.published_at is not null
  and p.published_at <= now()
group by p.id, p.slug, p.title, p.published_at, p.created_at
order by count(pc.blog_category_id) desc, p.published_at desc, p.created_at desc
limit 3;
