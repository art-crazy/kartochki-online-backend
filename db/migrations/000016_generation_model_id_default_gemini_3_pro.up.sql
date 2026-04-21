-- Меняем SQL default у model_id, чтобы база совпадала с доменным дефолтом.
-- Это важно для ручных вставок и аварийных сценариев, где значение может не прийти из приложения.
alter table generations
    alter column model_id set default 'google/gemini-3-pro-image-preview';
