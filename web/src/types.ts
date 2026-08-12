import { content } from './content';

export interface ProductCard {
  id: number;
  name: string;
  category: string;
  price: number; // минимальная цена
  old_price?: number; // цена до скидки (для бейджа −N%)
  image: string;
  is_hit?: boolean;
  stock?: number; // остаток (для бейджа «осталось N»)
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
  stock?: number;
  variants: ProductVariant[];
  images: ProductImage[];
}

/** Скидка в процентах по старой/новой цене (0, если скидки нет). */
export function discountPercent(price: number, oldPrice?: number): number {
  if (!oldPrice || oldPrice <= price) return 0;
  return Math.round((1 - price / oldPrice) * 100);
}

export interface OrderItem {
  id: number;
  variant_id: number;
  quantity: number;
  price: number;
  product_name: string;
  variant: ProductVariant;
}

/**
 * Способ доставки. Магазина и самовывоза нет:
 *   metro   — курьер отдаёт букет на станции, бесплатно (входит в цену букета);
 *   address — курьер по адресу, стоимость называет менеджер после заказа.
 * Стоимость доставки в приложении не существует: она не считается,
 * не хранится и никогда не попадает в сумму заказа.
 */
export const DELIVERY_TYPES = ['metro', 'address'] as const;
export type DeliveryType = (typeof DELIVERY_TYPES)[number];

export interface Order {
  id: number;
  total_price: number; // только букеты
  delivery_type: DeliveryType;
  metro_station: string;
  delivery_address: string;
  delivery_date: string;
  delivery_time: string;
  comment: string;
  status: string;
  items: OrderItem[];
  promo_code?: { code: string; discount_percent: number };
}

/**
 * Строка о доставке — одна на все экраны: подтверждение, история, уведомления.
 * Формулировки совпадают с админ-ботом (model.Order.DeliveryText).
 */
export function deliveryLine(o: Pick<Order, 'delivery_type' | 'metro_station'>): string {
  // Не-metro (включая заказы до появления выбора) описываем как курьерский:
  // лучше лишний раз сказать «менеджер свяжется», чем обещать бесплатное метро.
  if (o.delivery_type !== 'metro') return content.delivery.addressLine;
  return content.delivery.metroLine.replace('{station}', o.metro_station);
}

/** Время доставки для показа: пусто = клиент его не выбирал. */
export function deliveryTimeLabel(o: Pick<Order, 'delivery_type' | 'delivery_time'>): string {
  if (o.delivery_time) return o.delivery_time;
  return o.delivery_type === 'metro' ? content.delivery.timeNotSet : content.delivery.timeManaged;
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

export const CATEGORIES = [
  { emoji: '💚', name: 'Стандарт', image: '/seed/sunny-1.webp' },
  { emoji: '💎', name: 'Премиум', image: '/seed/roses-red-1.webp' },
  { emoji: '✨', name: 'Люкс', image: '/seed/peony-pastel-1.webp' },
  { emoji: '⚡', name: 'WOW', image: '/seed/heart-1.webp' },
] as const;

export const FILTERS = [
  { id: 'popular', label: '🔥 Популярное' },
  { id: 'new', label: '🆕 Новинки' },
  { id: 'preorder', label: '📅 Предзаказ' },
  { id: 'budget', label: '💰 До 3000 ₽' },
] as const;

/** Способы доставки: точное время выбирается отдельным полем (окно 9:00–21:00). */
export const DELIVERY_MODES = ['в течение часа', 'ко времени'] as const;
export type DeliveryMode = (typeof DELIVERY_MODES)[number];

export function formatPrice(p: number): string {
  return p.toLocaleString('ru-RU') + ' ₽';
}
