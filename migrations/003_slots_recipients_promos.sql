-- 003: слоты доставки, получатель/подарочный флоу, промокоды 2.0, допродажи,
-- идемпотентность заказов.
-- GORM AutoMigrate добавляет колонки и таблицы автоматически; перенос данных
-- старых промокодов выполняет cmd/main.go (DO-блок). Файл — справочный SQL-эквивалент.

-- Заказы: слот доставки хранится в delivery_time («10:00-12:00» … «20:00-22:00»),
-- получатель, скидка и ключ идемпотентности.
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS discount_amount      BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS recipient_name       TEXT,
    ADD COLUMN IF NOT EXISTS recipient_phone      TEXT,
    ADD COLUMN IF NOT EXISTS address_by_recipient BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS idempotency_key      TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS idx_orders_idempotency_key ON orders (idempotency_key);
-- Подсчёт занятости слотов (/api/delivery-slots и проверка при создании заказа).
CREATE INDEX IF NOT EXISTS idx_orders_ddate_dtime ON orders (delivery_date, delivery_time);

-- Товары-допродажи (ваза, открытка): блок «Добавить к заказу» в корзине.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS is_addon BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_products_is_addon ON products (is_addon);

-- Промокоды 2.0: percent|fixed, лимиты (общий и на пользователя), даты действия,
-- минимальная сумма, область действия, «первый заказ», персональная привязка.
ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS type              TEXT    NOT NULL DEFAULT 'percent',
    ADD COLUMN IF NOT EXISTS value             BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_uses          BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS used_count        BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS max_uses_per_user BIGINT  NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS starts_at         TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS expires_at        TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS is_active         BOOLEAN NOT NULL DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS min_order_amount  BIGINT  NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS applies_to        TEXT    NOT NULL DEFAULT 'all',
    ADD COLUMN IF NOT EXISTS first_order_only  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS bound_user_id     BIGINT REFERENCES users (id),
    ADD COLUMN IF NOT EXISTS auto_generated    BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS origin            TEXT    NOT NULL DEFAULT 'manual',
    ADD COLUMN IF NOT EXISTS created_at        TIMESTAMPTZ;

-- Перенос данных старой схемы и удаление её колонок.
UPDATE promo_codes SET type = 'percent', value = discount_percent WHERE value = 0;
UPDATE promo_codes SET used_count = uses WHERE used_count = 0;
ALTER TABLE promo_codes
    DROP COLUMN IF EXISTS discount_percent,
    DROP COLUMN IF EXISTS uses;

-- Применения промокодов: лимит «на пользователя» и статистика (/promo list, /promo info).
-- При отмене заказа до сборки запись удаляется, used_count уменьшается —
-- код снова доступен пользователю.
CREATE TABLE IF NOT EXISTS promo_redemptions (
    id            BIGSERIAL PRIMARY KEY,
    promo_code_id BIGINT NOT NULL REFERENCES promo_codes (id),
    user_id       BIGINT NOT NULL REFERENCES users (id),
    order_id      BIGINT NOT NULL UNIQUE REFERENCES orders (id),
    amount_saved  BIGINT NOT NULL,
    created_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_promo_redemptions_promo_user ON promo_redemptions (promo_code_id, user_id);
