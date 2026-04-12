# Database Draft

Этот файл собирает единую черновую схему БД на основе уже описанных page-контрактов.

Для удобного визуального просмотра ту же модель можно открыть в `dbdiagram.io` через файл [docs/database.dbml](/c:/Users/artcr/WebstormProjects/kartochki-online-all/kartochki-online-backend/docs/database.dbml:1).

Если нужен импорт в PostgreSQL через DataGrip, есть отдельный DDL-файл [docs/database.sql](/c:/Users/artcr/WebstormProjects/kartochki-online-all/kartochki-online-backend/docs/database.sql:1).

SQL-файл сейчас является более строгим проектным черновиком: в нём уже есть базовые `CHECK`-ограничения и явные внешние ключи.

Для БД это удобнее, чем держать связи по отдельным страницам:

- видно общую модель целиком
- легче замечать дублирование таблиц
- проще готовить миграции
- проще потом переносить это в `db/migrations` и `db/queries`

## Таблицы

| Таблица | Роль | Основной ключ | Важные внешние ключи |
| --- | --- | --- | --- |
| `users` | аккаунты пользователей | `id` | — |
| `sessions` | активные сессии входа | `id` | `user_id -> users.id` |
| `password_reset_tokens` | токены сброса пароля | `id` | `user_id -> users.id` |
| `oauth_accounts` | привязанные VK, Яндекс и другие OAuth-аккаунты | `id` | `user_id -> users.id` |
| `user_settings` | настройки профиля и генерации по умолчанию | `user_id` | `user_id -> users.id` |
| `notification_preferences` | переключатели уведомлений | `id` | `user_id -> users.id` |
| `api_keys` | API-ключи пользователя | `id` | `user_id -> users.id` |
| `plans` | тарифные планы | `id` | — |
| `subscriptions` | текущая или прошлые подписки пользователя | `id` | `user_id -> users.id`, `plan_id -> plans.id` |
| `usage_quotas` | лимиты и использование карточек по периоду | `id` | `user_id -> users.id`, `subscription_id -> subscriptions.id` |
| `addon_products` | разовые пакеты карточек | `id` | — |
| `payments` | платежи по тарифам и пакетам | `id` | `user_id -> users.id`, `subscription_id -> subscriptions.id?`, `addon_product_id -> addon_products.id?` |
| `projects` | пользовательские проекты генерации | `id` | `user_id -> users.id` |
| `assets` | исходные и итоговые файлы | `id` | `user_id -> users.id`, `project_id -> projects.id?` |
| `generation_jobs` | запуски генерации и их статус | `id` | `user_id -> users.id`, `project_id -> projects.id`, `source_asset_id -> assets.id`, `archive_asset_id -> assets.id?` |
| `generated_cards` | результат генерации по карточкам | `id` | `generation_job_id -> generation_jobs.id`, `asset_id -> assets.id` |
| `export_requests` | запросы на экспорт данных аккаунта | `id` | `user_id -> users.id`, `archive_asset_id -> assets.id?` |
| `account_deletion_requests` | запросы на удаление аккаунта | `id` | `user_id -> users.id` |
| `blog_posts` | статьи блога | `id` | `cover_asset_id -> assets.id?` |
| `blog_post_sections` | секции статьи | `id` | `blog_post_id -> blog_posts.id` |
| `blog_categories` | категории блога | `id` | — |
| `blog_post_categories` | связь статьи с категориями | `id` | `blog_post_id -> blog_posts.id`, `blog_category_id -> blog_categories.id` |
| `blog_tags` | теги блога | `id` | — |
| `blog_post_tags` | связь статьи с тегами | `id` | `blog_post_id -> blog_posts.id`, `blog_tag_id -> blog_tags.id` |
| `blog_post_metrics` | просмотры и служебные метрики статьи | `blog_post_id` | `blog_post_id -> blog_posts.id` |

## Таблица связей

| Родитель | Дочерняя таблица | Тип | Ключ связи | Зачем нужна связь |
| --- | --- | --- | --- | --- |
| `users` | `sessions` | `1:N` | `sessions.user_id` | пользователь может входить с нескольких устройств |
| `users` | `password_reset_tokens` | `1:N` | `password_reset_tokens.user_id` | несколько запросов на сброс пароля во времени |
| `users` | `oauth_accounts` | `1:N` | `oauth_accounts.user_id` | один пользователь может привязать несколько провайдеров |
| `users` | `user_settings` | `1:1` | `user_settings.user_id` | единый набор настроек профиля и дефолтов |
| `users` | `notification_preferences` | `1:N` | `notification_preferences.user_id` | несколько переключателей уведомлений |
| `users` | `api_keys` | `1:N` | `api_keys.user_id` | перевыпуск и история ключей |
| `users` | `subscriptions` | `1:N` | `subscriptions.user_id` | пользователь может менять тарифы во времени |
| `plans` | `subscriptions` | `1:N` | `subscriptions.plan_id` | подписка всегда относится к одному тарифу |
| `subscriptions` | `usage_quotas` | `1:N` | `usage_quotas.subscription_id` | лимиты считаются по периоду подписки |
| `users` | `usage_quotas` | `1:N` | `usage_quotas.user_id` | быстрый доступ к текущему использованию |
| `users` | `payments` | `1:N` | `payments.user_id` | платежи принадлежат пользователю |
| `subscriptions` | `payments` | `1:N` | `payments.subscription_id` | платежи по тарифу |
| `addon_products` | `payments` | `1:N` | `payments.addon_product_id` | платежи по разовым пакетам |
| `users` | `projects` | `1:N` | `projects.user_id` | у пользователя несколько проектов |
| `users` | `assets` | `1:N` | `assets.user_id` | файлы принадлежат пользователю |
| `projects` | `assets` | `1:N` | `assets.project_id` | итоговые файлы удобно привязывать к проекту |
| `users` | `generation_jobs` | `1:N` | `generation_jobs.user_id` | история всех запусков пользователя |
| `projects` | `generation_jobs` | `1:N` | `generation_jobs.project_id` | несколько запусков на один проект |
| `assets` | `generation_jobs` | `1:N` | `generation_jobs.source_asset_id` | запуск использует исходное изображение |
| `generation_jobs` | `generated_cards` | `1:N` | `generated_cards.generation_job_id` | один запуск создаёт набор карточек |
| `assets` | `generated_cards` | `1:1` или `1:N` | `generated_cards.asset_id` | у карточки есть итоговый файл |
| `users` | `export_requests` | `1:N` | `export_requests.user_id` | история запросов на экспорт |
| `users` | `account_deletion_requests` | `1:N` | `account_deletion_requests.user_id` | аудит запросов на удаление |
| `blog_posts` | `blog_post_sections` | `1:N` | `blog_post_sections.blog_post_id` | статья состоит из секций |
| `blog_posts` | `blog_post_metrics` | `1:1` | `blog_post_metrics.blog_post_id` | просмотры и counters по статье |
| `blog_posts` | `blog_post_categories` | `1:N` | `blog_post_categories.blog_post_id` | статья может входить в несколько категорий |
| `blog_categories` | `blog_post_categories` | `1:N` | `blog_post_categories.blog_category_id` | категория связывается со многими статьями |
| `blog_posts` | `blog_post_tags` | `1:N` | `blog_post_tags.blog_post_id` | статья может иметь много тегов |
| `blog_tags` | `blog_post_tags` | `1:N` | `blog_post_tags.blog_tag_id` | тег используется во многих статьях |

## Ключевые правила

- `user_settings` лучше держать отдельной таблицей `1:1`, а не раздувать `users`.
- `notification_preferences` лучше хранить строками по ключам, а не колонками на каждый toggle.
- `api_keys` лучше делать с историей и флагом активности, а не перезаписывать одно поле в `users`.
- `generation_jobs` и `generated_cards` должны быть отдельно, потому что запуск и результат — это разные сущности.
- `assets` лучше использовать и для исходников, и для результатов, чтобы не плодить две похожие таблицы.
- `blog_post_sections` лучше делать универсальной таблицей секций, а не отдельными таблицами под каждый вид блока.
- `payments` лучше держать одной таблицей и различать тип платежа полем, чем дробить на `subscription_payments` и `addon_payments`.
