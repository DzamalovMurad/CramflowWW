-- Схема БД CramFlow.
-- Приложение применяет её автоматически через GORM AutoMigrate при старте;
-- этот файл — справочный SQL-эквивалент (можно применить вручную: psql $DATABASE_URL -f ...).

CREATE TABLE IF NOT EXISTS products (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT,
    category    TEXT NOT NULL, -- Стандарт | Премиум | Люкс | WOW
    is_hidden   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_products_category  ON products (category);
CREATE INDEX IF NOT EXISTS idx_products_is_hidden ON products (is_hidden);

CREATE TABLE IF NOT EXISTS product_variants (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    quantity   BIGINT NOT NULL, -- количество цветов в букете
    price      BIGINT NOT NULL  -- цена в рублях
);
CREATE INDEX IF NOT EXISTS idx_product_variants_product_id ON product_variants (product_id);

CREATE TABLE IF NOT EXISTS product_images (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    url        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_product_images_product_id ON product_images (product_id);

CREATE TABLE IF NOT EXISTS promo_codes (
    id               BIGSERIAL PRIMARY KEY,
    code             TEXT NOT NULL UNIQUE,
    discount_percent BIGINT NOT NULL,
    uses             BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    telegram_id   BIGINT UNIQUE,
    name          TEXT,
    phone         TEXT,
    -- промокод, полученный по deep-link t.me/bot?start=CODE
    promo_code_id BIGINT REFERENCES promo_codes (id)
);

CREATE TABLE IF NOT EXISTS orders (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT NOT NULL REFERENCES users (id),
    total_price      BIGINT NOT NULL,
    delivery_address TEXT NOT NULL,
    delivery_date    TEXT NOT NULL,
    delivery_time    TEXT NOT NULL, -- 10:00-12:00 | 12:00-15:00 | 15:00-18:00
    promo_code_id    BIGINT REFERENCES promo_codes (id),
    comment          TEXT,
    status           TEXT NOT NULL DEFAULT 'new', -- new|confirmed|delivering|done|cancelled
    created_at       TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status  ON orders (status);

CREATE TABLE IF NOT EXISTS order_items (
    id           BIGSERIAL PRIMARY KEY,
    order_id     BIGINT NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    variant_id   BIGINT NOT NULL,
    quantity     BIGINT NOT NULL,
    price        BIGINT NOT NULL, -- цена за единицу на момент заказа
    product_name TEXT             -- название фиксируется на момент заказа
);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items (order_id);
