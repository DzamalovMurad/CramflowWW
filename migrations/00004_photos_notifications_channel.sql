-- Фото букета (file_id), отложенные уведомления, промокоды за отзыв/подписку,
-- источники трафика из startapp-параметра Mini App.
--
-- Ветка фото/уведомлений создавала эти объекты через GORM AutoMigrate и держала
-- рядом справочный 003_photos_notifications_channel.sql. После перехода на goose
-- источник правды один — этот файл. Номер 00004, а не 00003: ветка админ-бота
-- независимо от неё занумеровала свои изменения третьими, и обе «003» пришлось
-- развести, чтобы последовательность оставалась линейной и каждый файл
-- применялся ровно один раз.
--
-- orders.source здесь уже нет: колонку заводит 00003_admin_bot.sql (там она
-- NOT NULL DEFAULT 'direct'). Обе ветки писали в неё одно и то же — метку
-- канала привлечения, — поэтому колонка одна, а не две. Индекс по одному
-- source тоже не нужен: 00003 создаёт составной idx_orders_created_source,
-- а все отчёты режут заказы сначала по периоду.

-- +goose Up

-- Фото букета по заказу: храним только telegram file_id (без внешнего стораджа).
CREATE TABLE IF NOT EXISTS order_photos (
    id          BIGSERIAL PRIMARY KEY,
    order_id    BIGINT  NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    type        TEXT    NOT NULL, -- assembled | delivered
    file_id     TEXT    NOT NULL,
    is_document BOOLEAN NOT NULL DEFAULT FALSE, -- документ шлётся через sendDocument
    created_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_order_photos_order_id ON order_photos (order_id);

-- Отложенные уведомления: очередь живёт в БД (due_at), sent_at — guard от дублей,
-- перезапуск сервиса ничего не теряет. responded_at — клиент ответил на просьбу об отзыве.
CREATE TABLE IF NOT EXISTS notifications (
    id           BIGSERIAL PRIMARY KEY,
    order_id     BIGINT NOT NULL,
    type         TEXT   NOT NULL, -- feedback
    due_at       TIMESTAMPTZ NOT NULL,
    sent_at      TIMESTAMPTZ,
    responded_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_notifications_order_type ON notifications (order_id, type);
CREATE INDEX IF NOT EXISTS idx_notifications_due_at  ON notifications (due_at);
CREATE INDEX IF NOT EXISTS idx_notifications_sent_at ON notifications (sent_at);

-- Разовые бонусы (промокод за подписку): уникальный индекс — защита от фарминга.
CREATE TABLE IF NOT EXISTS claimed_bonuses (
    id            BIGSERIAL PRIMARY KEY,
    user_id       BIGINT NOT NULL,
    type          TEXT   NOT NULL, -- subscription
    promo_code_id BIGINT NOT NULL,
    created_at    TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_claimed_bonuses_user_type ON claimed_bonuses (user_id, type);

-- Одноразовые промокоды: max_uses = 1 (0 = без лимита, как раньше).
ALTER TABLE promo_codes
    ADD COLUMN IF NOT EXISTS max_uses BIGINT NOT NULL DEFAULT 0;

-- First-touch источник клиента (startapp-параметр первого запуска Mini App).
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS acquisition_source TEXT;

-- +goose Down

ALTER TABLE users
    DROP COLUMN IF EXISTS acquisition_source;

ALTER TABLE promo_codes
    DROP COLUMN IF EXISTS max_uses;

DROP TABLE IF EXISTS claimed_bonuses;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS order_photos;
