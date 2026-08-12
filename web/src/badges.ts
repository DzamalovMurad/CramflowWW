/**
 * Бейджи товара: что показать поверх фото и в каком порядке.
 *
 * Одно место на всё приложение — карточка каталога, страница товара, лента.
 * Иначе каждый экран решает сам, и на одном букете «ХИТ», а на другом
 * по тем же данным «АКЦИЯ».
 *
 * В сами изображения ничего не запекается: бейдж — слой интерфейса,
 * потому что акция живёт неделю, а фото — год.
 */
import { content } from './content';
import { discountPercent, isLowStock, type ProductCard } from './types';
import { hasSeasonBadge } from './seasonal';

export type BadgeVariant = 'stock' | 'daily' | 'sale' | 'season' | 'hit' | 'fresh';

export interface BadgeSpec {
  variant: BadgeVariant;
  label: string;
}

/** Товар в объёме, достаточном для бейджей: подходит и карточка, и деталка. */
type BadgeSource = Pick<ProductCard, 'is_hit' | 'is_fresh' | 'is_daily_pick' | 'stock'> & {
  price: number;
  old_price?: number;
  category?: string;
  name?: string;
};

/**
 * Порядок — от самого решающего к самому декоративному:
 *   осталось N — факт, который меняет решение прямо сейчас;
 *   букет дня  — редкий, один на весь магазин в сутки;
 *   акция      — деньги;
 *   сезон      — кампания, живёт неделями и только в своё окно;
 *   хит        — долгоиграющая пометка;
 *   свежее     — приятно, но само по себе не продаёт.
 */
export function productBadges(p: BadgeSource): BadgeSpec[] {
  const out: BadgeSpec[] = [];
  const off = discountPercent(p.price, p.old_price);

  if (isLowStock(p)) out.push({ variant: 'stock', label: `${content.badge.stock} ${p.stock}` });
  if (p.is_daily_pick) out.push({ variant: 'daily', label: content.badge.daily });
  if (off > 0) out.push({ variant: 'sale', label: `${content.badge.sale} −${off}%` });
  if (hasSeasonBadge(p as ProductCard)) out.push({ variant: 'season', label: content.seasonal.badge });
  if (p.is_hit) out.push({ variant: 'hit', label: content.badge.hit });
  if (p.is_fresh) out.push({ variant: 'fresh', label: content.badge.fresh });

  return out;
}

/**
 * Сколько бейджей показывать. В списке — один: сетка на 360px не выдерживает
 * стопку плашек, да и три «важных» пометки сразу не значат ничего.
 * На странице товара места больше — два.
 */
export function topBadges(p: BadgeSource, limit: number): BadgeSpec[] {
  return productBadges(p).slice(0, limit);
}
