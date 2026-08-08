import type { ProductCard } from './types';

/**
 * Сезонная кампания «к 1 сентября» — целиком на фронтенде: окно показа и отбор
 * букетов считаются в браузере. Backend, API и категории в БД не трогаем.
 * Чтобы выключить кампанию — достаточно вернуть false из isSeasonActive.
 */

/** Окно кампании: весь август и первые дни сентября. */
export function isSeasonActive(now: Date = new Date()): boolean {
  const month = now.getMonth();
  const day = now.getDate();
  if (month === 7) return true; // август — сборы к школе
  return month === 8 && day <= 5; // первые дни сентября
}

/** Классика школьного букета — ловим по названию (каталог ведут через админ-бота). */
const SEASON_KEYWORDS = [
  'гербер',
  'хризантем',
  'астр',
  'георгин',
  'гладиолус',
  'подсолн',
  'солнеч',
  'школ',
  'сентябр',
  'осен',
];

/** Ценовой диапазон «букета учителю» — подстраховка, если название нейтральное. */
const SEASON_MIN_PRICE = 1500;
const SEASON_MAX_PRICE = 4000;

/** Подходит ли букет под кампанию (без учёта даты). */
export function isSeasonPick(product: ProductCard): boolean {
  const name = product.name.toLowerCase();
  if (SEASON_KEYWORDS.some((k) => name.includes(k))) return true;
  return product.price >= SEASON_MIN_PRICE && product.price <= SEASON_MAX_PRICE;
}

/** Показывать ли сезонную плашку на карточке товара. */
export function hasSeasonBadge(product: ProductCard): boolean {
  return isSeasonActive() && isSeasonPick(product);
}

/** Отбор букетов кампании: вне окна показа — пустой список. */
export function seasonPicks(list: ProductCard[]): ProductCard[] {
  if (!isSeasonActive()) return [];
  return list.filter(isSeasonPick);
}
