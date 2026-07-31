-- Настройки рантайма и ручные бейджи витрины.
--
-- settings — key/value для переключателей, которые админ меняет из бота и которые
-- обязаны пережить рестарт контейнера (сейчас там живёт fallback-режим заказов).
-- products.low_stock — ручной бейдж «мало осталось» (в отличие от stock, который
-- считает штуки); products.sort_order — порядок в подборке хитов.

-- +goose Up

CREATE TABLE IF NOT EXISTS settings (
    key        TEXT NOT NULL PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ
);

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS low_stock  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS sort_order BIGINT  NOT NULL DEFAULT 0;

-- Подборка хитов для пустой корзины: сортировка по sort_order среди is_hit.
CREATE INDEX IF NOT EXISTS idx_products_hit_sort ON products (is_hit, sort_order);

-- Две колонки, которые AutoMigrate оставлял без DEFAULT/NOT NULL: на базах,
-- собранных им, orders.is_anonymous допускает NULL, и такая строка роняет
-- чтение заказа (в Go это обычный bool). Приводим существующие базы к тому же
-- виду, что и созданные с нуля миграцией 00001.
UPDATE orders SET is_anonymous = FALSE WHERE is_anonymous IS NULL;
ALTER TABLE orders
    ALTER COLUMN is_anonymous SET DEFAULT FALSE,
    ALTER COLUMN is_anonymous SET NOT NULL;

ALTER TABLE order_status_logs
    ALTER COLUMN admin_id SET DEFAULT 0;

-- +goose Down

DROP INDEX IF EXISTS idx_products_hit_sort;

ALTER TABLE products
    DROP COLUMN IF EXISTS low_stock,
    DROP COLUMN IF EXISTS sort_order;

DROP TABLE IF EXISTS settings;
