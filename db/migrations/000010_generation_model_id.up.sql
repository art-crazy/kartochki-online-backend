-- Добавляем поле model_id, чтобы worker знал, какую AI-модель использовать при обработке задачи.
-- Дефолт — первая и самая дешёвая модель. Существующие строки получат этот дефолт автоматически.
alter table generations
    add column model_id varchar not null default 'google/gemini-2.5-flash-image';
