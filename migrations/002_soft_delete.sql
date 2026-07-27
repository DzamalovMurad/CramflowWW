-- 002: soft delete товаров и вариантов.
-- Жёсткое удаление невозможно: order_items.variant_id ссылается на product_variants
-- (fk_order_items_variant), а история заказов должна оставаться читаемой.
-- GORM AutoMigrate добавляет эти же колонки автоматически; файл — SQL-эквивалент.

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

ALTER TABLE product_variants
    ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS old_price   BIGINT NOT NULL DEFAULT 0;

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS is_hit BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS stock  BIGINT  NOT NULL DEFAULT 0;

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS card_text     TEXT,
    ADD COLUMN IF NOT EXISTS is_anonymous  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS cancel_reason TEXT;

-- Витрина отдаёт только живые записи — индексы под эти выборки.
CREATE INDEX IF NOT EXISTS idx_products_archived_at         ON products (archived_at);
CREATE INDEX IF NOT EXISTS idx_product_variants_archived_at ON product_variants (archived_at);

-- Составные индексы под админ-CRM (заказы по статусу/дате и по клиенту).
CREATE INDEX IF NOT EXISTS idx_orders_status_ddate ON orders (status, delivery_date);
CREATE INDEX IF NOT EXISTS idx_orders_user_status  ON orders (user_id, status);

-- Журнал смен статусов заказов (разбор спорных ситуаций).
CREATE TABLE IF NOT EXISTS order_status_logs (
    id          BIGSERIAL PRIMARY KEY,
    order_id    BIGINT NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status TEXT   NOT NULL,
    to_status   TEXT   NOT NULL,
    admin_id    BIGINT NOT NULL DEFAULT 0, -- telegram_id админа (0 = система)
    created_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_order_status_logs_order_id ON order_status_logs (order_id);

-- «Сегодня на базе»: список свежих цветов за дату.
CREATE TABLE IF NOT EXISTS fresh_todays (
    id         BIGSERIAL PRIMARY KEY,
    date       TEXT NOT NULL UNIQUE, -- YYYY-MM-DD
    items      TEXT NOT NULL,
    created_at TIMESTAMPTZ
);
