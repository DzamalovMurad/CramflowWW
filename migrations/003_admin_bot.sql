-- 003: команды админ-бота /stats, /stock, /broadcast, /export.
-- GORM AutoMigrate добавляет те же колонки, таблицы и индексы автоматически;
-- файл — SQL-эквивалент для ручного применения (psql $DATABASE_URL -f ...).

-- --- Наличие товара (/stock) ---
-- Отдельно от is_hidden: is_hidden — сезонное «убрать с витрины»,
-- is_available — сегодняшняя закупка. Витрина требует обоих.
ALTER TABLE products
    ADD COLUMN IF NOT EXISTS is_available BOOLEAN NOT NULL DEFAULT TRUE;

-- Витрина всегда спрашивает один и тот же набор условий — частичный индекс
-- под него держит выборку каталога компактной независимо от размера архива.
CREATE INDEX IF NOT EXISTS idx_products_catalog
    ON products (created_at DESC)
    WHERE is_hidden = FALSE AND is_available = TRUE AND archived_at IS NULL;

-- --- Клиенты: рассылки и сегменты ---
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS bot_blocked  BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS last_cart_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS created_at   TIMESTAMPTZ;

-- «Новых клиентов за период» в /stats.
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users (created_at);
-- Сегмент «корзина без заказа»: активность корзины за 14 дней.
CREATE INDEX IF NOT EXISTS idx_users_last_cart_at ON users (last_cart_at);
-- Заблокировавшие бота исключаются из каждой выборки адресатов.
CREATE INDEX IF NOT EXISTS idx_users_bot_blocked ON users (bot_blocked);

-- --- Заказы: канал привлечения и получатель ---
ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS source          TEXT NOT NULL DEFAULT 'direct',
    ADD COLUMN IF NOT EXISTS recipient_name  TEXT,
    ADD COLUMN IF NOT EXISTS recipient_phone TEXT;

-- Сводка и выгрузка всегда режут заказы по created_at, а внутри периода
-- фильтруют по статусу и группируют по источнику — оба индекса составные,
-- первым полем идёт диапазон дат.
CREATE INDEX IF NOT EXISTS idx_orders_created_status ON orders (created_at, status);
CREATE INDEX IF NOT EXISTS idx_orders_created_source ON orders (created_at, source);

-- Топ товаров джойнит позиции к заказам периода.
CREATE INDEX IF NOT EXISTS idx_order_items_order_id ON order_items (order_id);

-- --- Рассылки (/broadcast) ---
CREATE TABLE IF NOT EXISTS broadcasts (
    id               BIGSERIAL PRIMARY KEY,
    admin_id         BIGINT NOT NULL,              -- telegram_id автора
    segment          TEXT   NOT NULL,              -- all | buyers | cart_no_order
    text             TEXT,                         -- текст или подпись к фото
    photo_id         TEXT,                         -- file_id фото в Telegram
    status           TEXT   NOT NULL DEFAULT 'draft', -- draft|queued|running|done|cancelled
    progress_chat_id BIGINT NOT NULL DEFAULT 0,    -- куда дорисовывать прогресс
    progress_msg_id  BIGINT NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ,
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ
);
-- Поиск незавершённых рассылок при старте сервиса.
CREATE INDEX IF NOT EXISTS idx_broadcasts_status ON broadcasts (status);

CREATE TABLE IF NOT EXISTS broadcast_recipients (
    id           BIGSERIAL PRIMARY KEY,
    broadcast_id BIGINT NOT NULL REFERENCES broadcasts (id) ON DELETE CASCADE,
    user_id      BIGINT NOT NULL REFERENCES users (id),
    telegram_id  BIGINT NOT NULL,
    -- pending → sending → sent | failed | blocked.
    -- sending выставляется ДО вызова Telegram: если сервис упадёт между
    -- отправкой и записью результата, такая строка не попадёт в повторную
    -- выборку и будет закрыта как failed — дубль в личке клиента хуже пропуска.
    status       TEXT   NOT NULL DEFAULT 'pending',
    error        TEXT,
    sent_at      TIMESTAMPTZ
);

-- Один адресат встречается в рассылке ровно один раз — гарантия уровня схемы,
-- а не аккуратности кода: повторное подтверждение не задвоит аудиторию.
CREATE UNIQUE INDEX IF NOT EXISTS idx_bcast_recipient_unique
    ON broadcast_recipients (broadcast_id, user_id);

-- Рабочая выборка отправщика: следующие pending этой рассылки + счётчики прогресса.
-- id третьим полем обязателен: адресаты берутся порциями в порядке id, и без
-- него к концу большой рассылки каждая порция перебирает всё уже отправленное.
CREATE INDEX IF NOT EXISTS idx_bcast_recipient_status
    ON broadcast_recipients (broadcast_id, status, id);
