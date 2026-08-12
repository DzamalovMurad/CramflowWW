-- 003: способ доставки в заказе.
-- Вариантов два: metro (до станции метро, бесплатно — входит в цену букета)
-- и address (курьер Яндекса, цену называет менеджер вручную).
-- Стоимость доставки в БД не хранится: её в системе нет, orders.total_price —
-- это всегда только букеты.
-- GORM AutoMigrate добавляет эти же колонки автоматически; файл — SQL-эквивалент.

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS delivery_type      TEXT   NOT NULL DEFAULT 'address', -- metro | address
    ADD COLUMN IF NOT EXISTS metro_station      TEXT,
    ADD COLUMN IF NOT EXISTS delivery_agreed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS delivery_agreed_by BIGINT NOT NULL DEFAULT 0; -- telegram_id админа

-- Заказы, оформленные до появления выбора, — всегда доставка по адресу.
UPDATE orders SET delivery_type = 'address' WHERE delivery_type IS NULL OR delivery_type = '';

-- Разбивка заказов по способу доставки в /stats.
CREATE INDEX IF NOT EXISTS idx_orders_delivery_type ON orders (delivery_type);

-- Адрес обязателен только для курьерской доставки: при доставке до метро он пуст.
ALTER TABLE orders ALTER COLUMN delivery_address SET DEFAULT '';
UPDATE orders SET delivery_address = '' WHERE delivery_address IS NULL;

-- Время доставки необязательно: при доставке до метро это пожелание клиента,
-- при курьерской — его согласует менеджер.
ALTER TABLE orders ALTER COLUMN delivery_time SET DEFAULT '';
UPDATE orders SET delivery_time = '' WHERE delivery_time IS NULL;
