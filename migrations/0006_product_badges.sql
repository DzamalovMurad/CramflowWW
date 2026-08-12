-- 0006: поля под бейджи витрины «СВЕЖЕЕ» и «БУКЕТ ДНЯ».
--
-- Почему не одна колонка badges TEXT[] на все бейджи: у каждого бейджа своё
-- правило жизни, и в общем списке их пришлось бы поддерживать руками.
--   «АКЦИЯ»     — уже выводится из product_variants.old_price, отдельного поля
--                 не нужно: иначе скидка окажется в двух местах и разойдётся.
--   «ХИТ»       — существующий products.is_hit, ставится вручную и живёт долго.
--   «СВЕЖЕЕ»    — про поставку, а не про товар: живёт часами. Храним срок,
--                 а не флаг, — бейдж гаснет сам, без уборки по расписанию.
--   «БУКЕТ ДНЯ» — ровно один товар на календарный день. Храним дату, а не флаг:
--                 частичный уникальный индекс физически не даст поставить
--                 второй букет дня на ту же дату, и вчерашний гаснет сам.

ALTER TABLE products ADD COLUMN IF NOT EXISTS fresh_until   TIMESTAMPTZ;
-- Дата в TEXT (YYYY-MM-DD) — как orders.delivery_date и fresh_todays.date:
-- календарный день магазина, а не момент времени, и без сюрпризов с часовым
-- поясом контейнера.
ALTER TABLE products ADD COLUMN IF NOT EXISTS daily_pick_on TEXT;

-- Один букет дня на дату. Частичный индекс: NULL-ов может быть сколько угодно.
CREATE UNIQUE INDEX IF NOT EXISTS idx_products_daily_pick
    ON products (daily_pick_on) WHERE daily_pick_on IS NOT NULL;

-- Витрина спрашивает «что свежее прямо сейчас» — индексируем только живые сроки.
CREATE INDEX IF NOT EXISTS idx_products_fresh_until
    ON products (fresh_until) WHERE fresh_until IS NOT NULL;

-- Формат даты проверяет БД: в колонку не должно попасть «сегодня» или «12.08».
ALTER TABLE products DROP CONSTRAINT IF EXISTS products_daily_pick_format;
ALTER TABLE products ADD CONSTRAINT products_daily_pick_format
    CHECK (daily_pick_on IS NULL OR daily_pick_on ~ '^\d{4}-\d{2}-\d{2}$');
