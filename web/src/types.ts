export interface ProductCard {
  id: number;
  name: string;
  category: string;
  price: number; // минимальная цена
  old_price?: number; // цена до скидки (для бейджа −N%)
  image: string;
  is_hit?: boolean;
  /** null = учёт остатка не ведётся, 0 = закончилось, N = осталось N. */
  stock: number | null;
  /** Свежая поставка. Срок считает сервер по времени магазина. */
  is_fresh?: boolean;
  /** Букет дня — ровно один товар на календарный день. */
  is_daily_pick?: boolean;
}

export interface ProductVariant {
  id: number;
  product_id: number;
  quantity: number; // цветов в букете
  price: number;
  old_price?: number; // цена до скидки
}

export interface ProductImage {
  id: number;
  product_id: number;
  url: string;
}

export interface Product {
  id: number;
  name: string;
  description: string;
  category: string;
  is_hit?: boolean;
  stock: number | null;
  is_fresh?: boolean;
  is_daily_pick?: boolean;
  variants: ProductVariant[];
  images: ProductImage[];
}

/** Товар можно заказать: либо остаток не ведётся, либо он положительный. */
export function inStock(p: { stock?: number | null }): boolean {
  return p.stock === null || p.stock === undefined || p.stock > 0;
}

/** Показывать ли бейдж «осталось N» — только когда остаток реально мал. */
export function isLowStock(p: { stock?: number | null }): boolean {
  return typeof p.stock === 'number' && p.stock > 0 && p.stock <= 5;
}

/** Скидка в процентах по старой/новой цене (0, если скидки нет). */
export function discountPercent(price: number, oldPrice?: number): number {
  if (!oldPrice || oldPrice <= price) return 0;
  return Math.round((1 - price / oldPrice) * 100);
}

export interface OrderItem {
  id: number;
  product_name: string;
  product_id: number;
  variant_id: number;
  flowers_count: number;
  quantity: number;
  price: number;
}

export interface Order {
  id: number;
  status: string;
  status_label: string;
  subtotal_price: number;
  discount_amount: number;
  total_price: number;
  delivery_address: string;
  delivery_date: string;
  delivery_time: string;
  comment: string;
  card_text: string;
  is_anonymous: boolean;
  cancel_reason?: string;
  created_at: string;
  items: OrderItem[];
  promo_code?: Promo;
}

/**
 * Промокод в том виде, в каком его отдаёт сервер.
 *
 * discount_percent остаётся для процентных кодов; у фиксированных он 0,
 * а размер скидки лежит в discount_value. Итог всё равно считает сервер —
 * здесь только предпросмотр, чтобы клиент видел ту же цифру заранее.
 */
export interface Promo {
  code: string;
  discount_percent: number;
  discount_type?: 'percent' | 'fixed';
  discount_value?: number;
  min_order_amount?: number;
}

/** Скидка в рублях от суммы корзины. Правило то же, что в model.Apply. */
export function promoDiscount(promo: Promo, subtotal: number): number {
  if (subtotal <= 0) return 0;
  if (promo.discount_type === 'fixed') {
    return Math.min(Math.max(promo.discount_value ?? 0, 0), subtotal);
  }
  return Math.floor((subtotal * Math.max(promo.discount_percent, 0)) / 100);
}

/** Подпись скидки: «15%» или «500 ₽». */
export function promoLabel(promo: Promo): string {
  if (promo.discount_type === 'fixed') return formatPrice(promo.discount_value ?? 0);
  return `${promo.discount_percent}%`;
}

/** Правила доставки, полученные от сервера (/api/config). */
export interface ShopConfig {
  today: string;
  open_hour: number;
  close_hour: number;
  /** Экспресс-доставка возможна прямо сейчас. */
  express_available: boolean;
  /** Самое раннее время «ко времени» на сегодня; '' — сегодня уже не успеть. */
  earliest_today: string;
  max_preorder_days: number;
}

export interface CartItem {
  variantId: number;
  productId: number;
  productName: string;
  flowersCount: number; // цветов в букете (из варианта)
  price: number;
  image: string;
  qty: number;
}

/** Категории витрины. Значения обязаны совпадать с model.Categories на сервере. */
export const CATEGORIES = ['Стандарт', 'Премиум', 'Люкс', 'WOW'] as const;

export function formatPrice(p: number): string {
  return p.toLocaleString('ru-RU') + ' ₽';
}

const MONTHS = [
  'января', 'февраля', 'марта', 'апреля', 'мая', 'июня',
  'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря',
];

/** Дата YYYY-MM-DD → «5 августа»: для человека, а не для сервера. */
export function formatDate(iso: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (!m) return iso;
  const month = MONTHS[Number(m[2]) - 1];
  return month ? `${Number(m[3])} ${month}` : iso;
}

/** Сдвиг даты на N дней от YYYY-MM-DD. Считаем по локальному календарю,
 *  чтобы «завтра» не уезжало на сутки из-за часового пояса. */
export function shiftDate(iso: string, days: number): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(iso);
  if (!m) return iso;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  d.setDate(d.getDate() + days);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

/** Час → «9:00» для подписей окна доставки. */
export function formatHour(hour: number): string {
  return `${hour}:00`;
}
