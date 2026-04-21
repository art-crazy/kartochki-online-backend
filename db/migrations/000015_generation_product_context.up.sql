-- Контекст товара для генерации карточек.
-- Хранит название, описание, преимущества и характеристики товара,
-- которые были переданы пользователем при запуске генерации.
-- Данные используются prompt builder для создания детального арт-директорского ТЗ.
CREATE TABLE generation_product_context (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    generation_id   UUID NOT NULL REFERENCES generations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    category        TEXT,
    brand           TEXT,
    description     TEXT,
    benefits        TEXT[] NOT NULL DEFAULT '{}',
    characteristics JSONB NOT NULL DEFAULT '[]',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
