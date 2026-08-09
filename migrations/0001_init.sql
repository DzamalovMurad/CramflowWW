-- 0001 — полная схема Flowix.
--
-- Единственный механизм миграций в проекте: файлы из этой папки применяет
-- internal/migrate поштучно, каждый в своей транзакции, отмечая версию
-- в schema_migrations. GORM AutoMigrate НЕ используется — схема здесь.
--
-- Файл идемпотентен (IF NOT EXISTS везде): его безопасно применить и к пустой
-- базе, и к базе, схему которой когда-то создавал AutoMigrate.

-- ─── Товары ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS products (
    id          BIGSERIAL PRIMARY KEY,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    category    TEXT        NOT NULL,
    is_hidden   BOOLEAN     NOT NULL DEFAULT FALSE,
    is_hit      BOOLEAN     NOT NULL DEFAULT FALSE,
    -- stock NULL = количество не ограничено; число = реальный остаток,
    -- он уменьшается при заказе, возвращается при отмене, и 0 значит
    -- «закончилось». Отдельные значения для «не ведём учёт» и «ноль штук»
    -- обязательны: с общим нулём остаток никогда бы не заканчивался.
    stock       INTEGER,
    archived_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE products ADD COLUMN IF NOT EXISTS description TEXT        NOT NULL DEFAULT '';
ALTER TABLE products ADD COLUMN IF NOT EXISTS is_hit      BOOLEAN     NOT NULL DEFAULT FALSE;
ALTER TABLE products ADD COLUMN IF NOT EXISTS stock       INTEGER;
ALTER TABLE products ALTER COLUMN stock DROP NOT NULL;
ALTER TABLE products ALTER COLUMN stock DROP DEFAULT;
-- В прежней схеме 0 означал «не ограничивать» — переносим этот смысл в NULL.
UPDATE products SET stock = NULL WHERE stock = 0;
-- Postgres не умеет ADD CONSTRAINT IF NOT EXISTS — оборачиваем в DO-блок,
-- чтобы файл оставался идемпотентным.
DO $$
BEGIN
    ALTER TABLE products ADD CONSTRAINT products_stock_non_negative
        CHECK (stock IS NULL OR stock >= 0);
EXCEPTION
    WHEN duplicate_object THEN NULL;
END
$$;
ALTER TABLE products ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE products ADD COLUMN IF NOT EXISTS created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_products_category ON products (category);
-- Главный запрос витрины: живые товары, новые сверху. Частичный индекс
-- покрывает и фильтр, и сортировку.
CREATE INDEX IF NOT EXISTS idx_products_visible
    ON products (created_at DESC)
    WHERE is_hidden = FALSE AND archived_at IS NULL;

CREATE TABLE IF NOT EXISTS product_variants (
    id          BIGSERIAL PRIMARY KEY,
    product_id  BIGINT  NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    quantity    INTEGER NOT NULL CHECK (quantity > 0),  -- цветов в букете
    price       INTEGER NOT NULL CHECK (price > 0),     -- целые рубли
    old_price   INTEGER NOT NULL DEFAULT 0 CHECK (old_price >= 0),
    archived_at TIMESTAMPTZ
);
ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS old_price   INTEGER NOT NULL DEFAULT 0;
ALTER TABLE product_variants ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_product_variants_product_id ON product_variants (product_id);
-- Живые варианты товара, отсортированные по цене (карточка и витрина).
CREATE INDEX IF NOT EXISTS idx_product_variants_live
    ON product_variants (product_id, price)
    WHERE archived_at IS NULL;

CREATE TABLE IF NOT EXISTS product_images (
    id         BIGSERIAL PRIMARY KEY,
    product_id BIGINT NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    url        TEXT   NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_product_images_product_id ON product_images (product_id, id);

-- ─── Промокоды ─────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS promo_codes (
    id               BIGSERIAL PRIMARY KEY,
    code             TEXT    NOT NULL UNIQUE,
    discount_percent INTEGER NOT NULL CHECK (discount_percent BETWEEN 1 AND 90),
    uses             INTEGER NOT NULL DEFAULT 0,
    -- 0 = без ограничения
    max_uses         INTEGER NOT NULL DEFAULT 0 CHECK (max_uses >= 0),
    -- сколько раз одному клиенту; 0 = без ограничения
    per_user_limit   INTEGER NOT NULL DEFAULT 1 CHECK (per_user_limit >= 0),
    is_active        BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at       TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS max_uses       INTEGER     NOT NULL DEFAULT 0;
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS per_user_limit INTEGER     NOT NULL DEFAULT 1;
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS is_active      BOOLEAN     NOT NULL DEFAULT TRUE;
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS expires_at     TIMESTAMPTZ;
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Регистронезависимый поиск кода без последовательного скана.
CREATE UNIQUE INDEX IF NOT EXISTS idx_promo_codes_upper ON promo_codes (UPPER(code));

-- ─── Клиенты ───────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    telegram_id   BIGINT      NOT NULL UNIQUE,
    name          TEXT        NOT NULL DEFAULT '',
    phone         TEXT        NOT NULL DEFAULT '',
    -- промокод, полученный по deep-link ?start=CODE
    promo_code_id BIGINT      REFERENCES promo_codes (id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE users ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- ─── Заказы ────────────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS orders (
    id               BIGSERIAL PRIMARY KEY,
    user_id          BIGINT  NOT NULL REFERENCES users (id),
    -- Деньги: только целые рубли. total = subtotal - discount.
    subtotal_price   INTEGER NOT NULL DEFAULT 0 CHECK (subtotal_price >= 0),
    discount_amount  INTEGER NOT NULL DEFAULT 0 CHECK (discount_amount >= 0),
    total_price      INTEGER NOT NULL CHECK (total_price >= 0),
    delivery_address TEXT    NOT NULL,
    delivery_date    TEXT    NOT NULL,   -- YYYY-MM-DD в часовом поясе магазина
    delivery_time    TEXT    NOT NULL,
    recipient_name   TEXT    NOT NULL DEFAULT '', -- пусто = получатель сам заказчик
    recipient_phone  TEXT    NOT NULL DEFAULT '',
    promo_code_id    BIGINT  REFERENCES promo_codes (id) ON DELETE SET NULL,
    comment          TEXT    NOT NULL DEFAULT '',
    card_text        TEXT    NOT NULL DEFAULT '',
    is_anonymous     BOOLEAN NOT NULL DEFAULT FALSE,
    cancel_reason    TEXT    NOT NULL DEFAULT '',
    status           TEXT    NOT NULL DEFAULT 'new',
    -- ключ идемпотентности из заголовка Idempotency-Key: повторная отправка
    -- той же формы возвращает уже созданный заказ, а не создаёт второй
    idempotency_key  TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE orders ADD COLUMN IF NOT EXISTS subtotal_price  INTEGER     NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS discount_amount INTEGER     NOT NULL DEFAULT 0;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS recipient_name  TEXT        NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS recipient_phone TEXT        NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS card_text       TEXT        NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS is_anonymous    BOOLEAN     NOT NULL DEFAULT FALSE;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS cancel_reason   TEXT        NOT NULL DEFAULT '';
ALTER TABLE orders ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE orders ADD COLUMN IF NOT EXISTS updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE INDEX IF NOT EXISTS idx_orders_user_id    ON orders (user_id, id DESC);
CREATE INDEX IF NOT EXISTS idx_orders_status     ON orders (status, id DESC);
-- Предзаказы: ведущая колонка — дата, потому что запрос фильтрует по ней.
CREATE INDEX IF NOT EXISTS idx_orders_delivery   ON orders (delivery_date, delivery_time);
CREATE INDEX IF NOT EXISTS idx_orders_created_at ON orders (created_at DESC);
-- Идемпотентность: второй заказ с тем же ключом физически невозможен.
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency
    ON orders (idempotency_key) WHERE idempotency_key IS NOT NULL;

CREATE TABLE IF NOT EXISTS order_items (
    id           BIGSERIAL PRIMARY KEY,
    order_id     BIGINT  NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    variant_id   BIGINT  NOT NULL REFERENCES product_variants (id),
    quantity     INTEGER NOT NULL CHECK (quantity > 0),
    price        INTEGER NOT NULL CHECK (price >= 0), -- цена за штуку на момент заказа
    product_name TEXT    NOT NULL DEFAULT ''
);
ALTER TABLE order_items ALTER COLUMN product_name SET DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_order_items_order_id   ON order_items (order_id);
-- Нужен и «популярному» (JOIN по variant_id), и подчистке архивных вариантов.
CREATE INDEX IF NOT EXISTS idx_order_items_variant_id ON order_items (variant_id);

-- Списания промокодов: по этой таблице считаются лимиты max_uses и per_user_limit.
CREATE TABLE IF NOT EXISTS promo_redemptions (
    id            BIGSERIAL PRIMARY KEY,
    promo_code_id BIGINT      NOT NULL REFERENCES promo_codes (id) ON DELETE CASCADE,
    user_id       BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    order_id      BIGINT      NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_promo_redemptions_order
    ON promo_redemptions (order_id);
CREATE INDEX IF NOT EXISTS idx_promo_redemptions_user
    ON promo_redemptions (promo_code_id, user_id);

-- Журнал смен статусов — разбор спорных ситуаций.
CREATE TABLE IF NOT EXISTS order_status_logs (
    id          BIGSERIAL PRIMARY KEY,
    order_id    BIGINT      NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status TEXT        NOT NULL,
    to_status   TEXT        NOT NULL,
    admin_id    BIGINT      NOT NULL DEFAULT 0, -- telegram_id админа, 0 = система
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_order_status_logs_order_id ON order_status_logs (order_id, id);

-- ─── Прочее ────────────────────────────────────────────────────────────────

-- «Сегодня на базе»: что флорист закупил утром.
CREATE TABLE IF NOT EXISTS fresh_todays (
    id         BIGSERIAL PRIMARY KEY,
    date       TEXT        NOT NULL UNIQUE, -- YYYY-MM-DD
    items      TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Фото товаров, если UPLOAD_STORE=db (Railway без Volume).
CREATE TABLE IF NOT EXISTS uploads (
    id         BIGSERIAL PRIMARY KEY,
    ext        TEXT        NOT NULL,
    mime_type  TEXT        NOT NULL,
    data       BYTEA       NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
