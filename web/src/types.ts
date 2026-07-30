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

export interface Order {
  id: number;
  total_price: number; // к оплате (после скидки)
  discount_amount?: number;
  delivery_address: string;
  delivery_date: string;
  delivery_time: string; // слот «10:00-12:00» … «20:00-22:00»
  comment: string;
  recipient_name?: string;
  recipient_phone?: string;
  address_by_recipient?: boolean;
  card_text?: string;
  is_anonymous?: boolean;
  status: string;
  created_at?: string;
  items: OrderItem[];
  promo_code?: { code: string; type?: string; value?: number };
}

/** Статусы заказа для истории в профиле (строчными — стиль бренда). */
export const STATUS_LABELS: Record<string, string> = {
  new: 'новый',
  confirmed: 'подтверждён',
  assembling: 'собираем',
  photo_sent: 'фото отправлено',
  delivering: 'в пути',
  delivered: 'доставлен',
  cancelled: 'отменён',
};

/** Ответ сервера на проверку промокода: скидка уже рассчитана по корзине. */
export interface PromoInfo {
  code: string;
  type: 'percent' | 'fixed';
  value: number;
  label: string; // «10%» или «500 ₽»
  discount: number; // ₽ для текущей корзины
}

/** Слот доставки из /api/delivery-slots. */
export interface SlotInfo {
  slot: string;
  available: boolean;
  reason?: string; // «уже недоступен» | «занят»
}

export interface SlotDay {
  date: string; // YYYY-MM-DD
  label: string; // «сегодня» | «завтра» | «сб, 2 авг»
  slots: SlotInfo[];
}

/** Товар-допродажа для блока «Добавить к заказу» в корзине. */
export interface AddonCard {
  id: number;
  name: string;
  price: number;
  image: string;
  variant_id: number;
  flowers_count: number;
}

/** Позиция «повторить заказ»: актуальный вариант либо причина недоступности. */
export interface RepeatItem {
  available: boolean;
  reason?: string;
  product_id?: number;
  product_name: string;
  variant_id?: number;
  flowers_count?: number;
  price?: number;
  image?: string;
  qty: number;
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

export function formatPrice(p: number): string {
  return p.toLocaleString('ru-RU') + ' ₽';
}
