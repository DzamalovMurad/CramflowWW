export interface ProductCard {
  id: number;
  name: string;
  category: string;
  price: number; // минимальная цена
  old_price?: number; // цена до скидки (для бейджа −N%)
  image: string;
  is_hit?: boolean; // бейдж «хит»
  low_stock?: boolean; // ручной бейдж «мало осталось»
  seasonal?: boolean; // товар есть в «сегодня на базе» → бейдж «сезонный»
  stock?: number; // остаток (для бейджа «осталось N»)
}

/** Конфиг развёрнутого сервиса (/api/config). */
export interface AppConfig {
  bot_username: string;
  fallback_orders: boolean;
  mini_app_url: string;
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
  low_stock?: boolean;
  seasonal?: boolean;
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

export interface Order {
  id: number;
  total_price: number;
  delivery_address: string;
  delivery_date: string;
  delivery_time: string;
  comment: string;
  status: string;
  items: OrderItem[];
  promo_code?: { code: string; discount_percent: number };
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
