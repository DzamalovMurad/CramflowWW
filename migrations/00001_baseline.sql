-- Базовая схема CramFlow.
--
-- Эта миграция — слепок схемы, которую до перехода на goose создавал
-- GORM AutoMigrate (плюс прежние справочные 001_init.sql и 002_soft_delete.sql).
-- Все операции идемпотентны (IF NOT EXISTS), поэтому миграция одинаково
-- корректно применяется и к пустой базе, и к уже работающей проде,
-- собранной AutoMigrate: там она лишь проставит запись в goose_db_version.
--
-- Имена индексов и внешних ключей намеренно повторяют те, что генерирует GORM
-- (fk_orders_user, idx_users_telegram_id и т.д.). Иначе база, созданная с нуля,
-- отличалась бы от продовой именами объектов — и любой инструмент, который на
-- них смотрит, вёл бы себя в двух окружениях по-разному.
--
-- Уникальность — через UNIQUE INDEX, а не UNIQUE-констрейнт: AutoMigrate делал
-- именно индексы (idx_promo_codes_code), и в pg_constraint их нет.

-- +goose Up

CREATE TABLE IF NOT EXISTS products (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT    NOT NULL,
    description TEXT,
    category    TEXT    NOT NULL,                  -- Стандарт | Премиум | Люкс | WOW
    is_hidden   BOOLEAN NOT NULL DEFAULT FALSE,
    is_hit      BOOLEAN NOT NULL DEFAULT FALSE,    -- бейдж «хит»
    stock       BIGINT  NOT NULL DEFAULT 0,        -- остаток (0 = бейдж не показывать)
    archived_at TIMESTAMPTZ,                       -- soft delete: убран с витрины навсегда
    created_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_products_category    ON products (category);
CREATE INDEX IF NOT EXISTS idx_products_is_hidden   ON products (is_hidden);
CREATE INDEX IF NOT EXISTS idx_products_archived_at ON products (archived_at);

CREATE TABLE IF NOT EXISTS product_variants (
    id          BIGSERIAL PRIMARY KEY,
    product_id  BIGINT NOT NULL,
    quantity    BIGINT NOT NULL,            -- цветов в букете
    price       BIGINT NOT NULL,            -- цена в рублях
    old_price   BIGINT NOT NULL DEFAULT 0,  -- цена до скидки (0 = без скидки)
    archived_at TIMESTAMPTZ,                -- заменён при правке цен, но нужен истории заказов
    CONSTRAINT fk_products_variants FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_product_variants_product_id  ON product_variants (product_id);
CREATE INDEX IF NOT EXISTS idx_product_variants_archived_at ON product_variants (archived_at);

CREATE TABLE IF NOT EXISTS product_images (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL,
    url        TEXT   NOT NULL,
    CONSTRAINT fk_products_images FOREIGN KEY (product_id) REFERENCES products (id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_product_images_product_id ON product_images (product_id);

CREATE TABLE IF NOT EXISTS promo_codes (
    id               BIGSERIAL PRIMARY KEY,
    code             TEXT   NOT NULL,
    discount_percent BIGINT NOT NULL,
    uses             BIGINT NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_promo_codes_code ON promo_codes (code);

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    telegram_id   BIGINT,
    name          TEXT,
    phone         TEXT,
    promo_code_id BIGINT, -- промокод из deep-link t.me/bot?start=CODE
    CONSTRAINT fk_users_promo_code FOREIGN KEY (promo_code_id) REFERENCES promo_codes (id)
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_telegram_id ON users (telegram_id);

CREATE TABLE IF NOT EXISTS orders (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT  NOT NULL,
    total_price      BIGINT  NOT NULL,
    delivery_address TEXT    NOT NULL,
    delivery_date    TEXT    NOT NULL,
    delivery_time    TEXT    NOT NULL,
    promo_code_id    BIGINT,
    comment          TEXT,
    card_text        TEXT,                            -- текст открытки (до 300 символов)
    is_anonymous     BOOLEAN NOT NULL DEFAULT FALSE,
    cancel_reason    TEXT,
    status           TEXT    NOT NULL DEFAULT 'new',  -- см. model.StatusOrder
    created_at       TIMESTAMPTZ,
    CONSTRAINT fk_orders_user       FOREIGN KEY (user_id)       REFERENCES users (id),
    CONSTRAINT fk_orders_promo_code FOREIGN KEY (promo_code_id) REFERENCES promo_codes (id)
);
CREATE INDEX IF NOT EXISTS idx_orders_user_id      ON orders (user_id);
CREATE INDEX IF NOT EXISTS idx_orders_status       ON orders (status);
CREATE INDEX IF NOT EXISTS idx_orders_status_ddate ON orders (status, delivery_date);
CREATE INDEX IF NOT EXISTS idx_orders_user_status  ON orders (status, user_id);

CREATE TABLE IF NOT EXISTS order_items (
    id           BIGSERIAL PRIMARY KEY,
    order_id     BIGINT NOT NULL,
    variant_id   BIGINT NOT NULL,
    quantity     BIGINT NOT NULL,
    price        BIGINT NOT NULL, -- цена за единицу на момент заказа
    product_name TEXT,            -- название фиксируется на момент заказа
    CONSTRAINT fk_orders_items       FOREIGN KEY (order_id)   REFERENCES orders (id) ON DELETE CASCADE,
    CONSTRAINT fk_order_items_variant FOREIGN KEY (variant_id) REFERENCES product_variants (id)
);
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items (order_id);

-- Журнал смен статусов заказов (разбор спорных ситуаций).
-- Внешнего ключа нет намеренно: в модели связи с Order тоже нет, только индекс.
CREATE TABLE IF NOT EXISTS order_status_logs (
    id          BIGSERIAL PRIMARY KEY,
    order_id    BIGINT NOT NULL,
    from_status TEXT   NOT NULL,
    to_status   TEXT   NOT NULL,
    admin_id    BIGINT NOT NULL DEFAULT 0, -- telegram_id админа (0 = система)
    created_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_order_status_logs_order_id ON order_status_logs (order_id);

-- «Сегодня на базе»: что флорист закупил утром. Отсюда же берётся бейдж «сезонный».
CREATE TABLE IF NOT EXISTS fresh_todays (
    id         BIGSERIAL PRIMARY KEY,
    date       TEXT NOT NULL, -- YYYY-MM-DD
    items      TEXT NOT NULL, -- «пионы, ранункулюсы, эустома»
    created_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_fresh_todays_date ON fresh_todays (date);

-- Фото товаров при UPLOAD_STORE=db (хостинг без постоянного диска).
CREATE TABLE IF NOT EXISTS uploads (
    id         BIGSERIAL PRIMARY KEY,
    ext        TEXT  NOT NULL,
    mime_type  TEXT  NOT NULL,
    data       BYTEA NOT NULL,
    created_at TIMESTAMPTZ
);

-- +goose Down

DROP TABLE IF EXISTS uploads;
DROP TABLE IF EXISTS fresh_todays;
DROP TABLE IF EXISTS order_status_logs;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS promo_codes;
DROP TABLE IF EXISTS product_images;
DROP TABLE IF EXISTS product_variants;
DROP TABLE IF EXISTS products;
