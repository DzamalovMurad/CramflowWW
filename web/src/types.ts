export interface ProductCard {
  id: number;
  name: string;
  category: string;
  price: number; // минимальная цена
  image: string;
}

export interface ProductVariant {
  id: number;
  product_id: number;
  quantity: number; // цветов в букете
  price: number;
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
  variants: ProductVariant[];
  images: ProductImage[];
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
  { emoji: '💚', name: 'Стандарт' },
  { emoji: '💎', name: 'Премиум' },
  { emoji: '✨', name: 'Люкс' },
  { emoji: '⚡', name: 'WOW' },
] as const;

export const FILTERS = [
  { id: 'popular', label: '🔥 Популярное' },
  { id: 'new', label: '🆕 Новинки' },
  { id: 'today', label: '🚚 Доставка сегодня' },
  { id: 'budget', label: '💰 До 3000 ₽' },
] as const;

export const TIME_SLOTS = ['10:00-12:00', '12:00-15:00', '15:00-18:00'] as const;

export function formatPrice(p: number): string {
  return p.toLocaleString('ru-RU') + ' ₽';
}
