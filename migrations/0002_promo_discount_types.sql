-- Промокоды: фиксированная скидка и минимальная сумма заказа.
--
-- До этой миграции скидка была только процентной и жила в одной колонке
-- promo_codes.discount_percent. Теперь правило скидки описывают две колонки:
-- discount_type ('percent' | 'fixed') и discount_value (проценты или рубли).
-- Держать рядом старую discount_percent нельзя: два источника правды про одну
-- и ту же скидку рано или поздно разъезжаются, а это прямые деньги.

ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS discount_type    TEXT    NOT NULL DEFAULT 'percent';
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS discount_value   INTEGER NOT NULL DEFAULT 0;
-- 0 = минимальной суммы нет
ALTER TABLE promo_codes ADD COLUMN IF NOT EXISTS min_order_amount INTEGER NOT NULL DEFAULT 0;

-- Перенос существующих процентов и снятие старой колонки — одним шагом,
-- чтобы миграцию можно было прогнать повторно на уже мигрированной базе.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'promo_codes' AND column_name = 'discount_percent'
    ) THEN
        UPDATE promo_codes
           SET discount_type  = 'percent',
               discount_value = discount_percent
         WHERE discount_value = 0;
        ALTER TABLE promo_codes DROP COLUMN discount_percent;
    END IF;
END $$;

ALTER TABLE promo_codes DROP CONSTRAINT IF EXISTS promo_codes_discount_type_check;
ALTER TABLE promo_codes ADD  CONSTRAINT promo_codes_discount_type_check
    CHECK (discount_type IN ('percent', 'fixed'));

-- Процент ограничен 90: скидка «в ноль» — это почти всегда ошибка ввода,
-- а не акция. Фиксированная скидка ограничена сверху ценой самого дорогого
-- букета: больше — тоже опечатка.
ALTER TABLE promo_codes DROP CONSTRAINT IF EXISTS promo_codes_discount_value_check;
ALTER TABLE promo_codes ADD  CONSTRAINT promo_codes_discount_value_check CHECK (
       (discount_type = 'percent' AND discount_value BETWEEN 1 AND 90)
    OR (discount_type = 'fixed'   AND discount_value BETWEEN 1 AND 1000000)
);

ALTER TABLE promo_codes DROP CONSTRAINT IF EXISTS promo_codes_min_order_amount_check;
ALTER TABLE promo_codes ADD  CONSTRAINT promo_codes_min_order_amount_check
    CHECK (min_order_amount >= 0);

-- Снимок применённого кода в самом заказе. promo_code_id стоит ON DELETE SET
-- NULL: если акцию удалят, у заказа осталась бы скидка без объяснения, откуда
-- она взялась. Текст кода фиксируем на момент оформления — как и название
-- товара в позиции заказа.
ALTER TABLE orders ADD COLUMN IF NOT EXISTS applied_promo_code TEXT NOT NULL DEFAULT '';

UPDATE orders o
   SET applied_promo_code = p.code
  FROM promo_codes p
 WHERE o.promo_code_id = p.id
   AND o.applied_promo_code = '';
