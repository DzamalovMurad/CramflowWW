import { content } from './content';

/**
 * Бейджи товара — один расчёт на карточку каталога и на страницу товара,
 * чтобы витрина не противоречила сама себе.
 *
 * Классы (`badge-*`) описаны в styles/index.css и используют уже существующие
 * цвета: новых оттенков ради бейджей не заводим.
 */
export interface Badge {
  key: string;
  label: string;
  cls: string;
}

interface BadgeSource {
  is_hit?: boolean;
  low_stock?: boolean;
  seasonal?: boolean;
  stock?: number;
}

/** lowStockThreshold — с какого остатка показываем «осталось N». */
const lowStockThreshold = 5;

/**
 * productBadges собирает плашки по убыванию важности для покупателя.
 * off — скидка в процентах (0, если её нет).
 */
export function productBadges(p: BadgeSource, off = 0): Badge[] {
  const b = content.badges;
  const badges: Badge[] = [];

  if (p.is_hit) badges.push({ key: 'hit', label: b.hit, cls: 'badge-hit' });
  if (off > 0) badges.push({ key: 'sale', label: `−${off}%`, cls: 'badge-sale' });

  // Ручной флаг важнее счётчика: флорист ставит его, когда видит остаток
  // на базе, а точное число в карточке держат не всегда.
  const counted = p.stock !== undefined && p.stock > 0 && p.stock <= lowStockThreshold;
  if (p.low_stock) {
    badges.push({ key: 'low', label: b.lowStock, cls: 'badge-low' });
  } else if (counted) {
    badges.push({ key: 'stock', label: `осталось ${p.stock}`, cls: 'badge-stock' });
  }

  if (p.seasonal) badges.push({ key: 'seasonal', label: b.seasonal, cls: 'badge-seasonal' });

  return badges;
}
