import type { Order, Product, ProductCard } from './types';
import { initDataHeader } from './telegram';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...initDataHeader(),
      ...init?.headers,
    },
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(body.error || 'Что-то пошло не так, попробуйте ещё раз');
  }
  return body as T;
}

export function fetchProducts(category: string, filter: string, search = ''): Promise<ProductCard[]> {
  const params = new URLSearchParams();
  if (category) params.set('category', category);
  if (filter) params.set('filter', filter);
  if (search) params.set('q', search);
  const qs = params.toString();
  return request(`/api/products${qs ? `?${qs}` : ''}`);
}

export function fetchProduct(id: number | string): Promise<Product> {
  return request(`/api/products/${id}`);
}

export interface OrderPayload {
  items: { variant_id: number; quantity: number }[];
  name: string;
  phone: string;
  delivery_address: string;
  delivery_date: string;
  delivery_time: string;
  comment: string;
  card_text: string;
  is_anonymous: boolean;
  promo_code: string;
}

export function createOrder(payload: OrderPayload): Promise<Order> {
  return request('/api/orders', { method: 'POST', body: JSON.stringify(payload) });
}

export function checkPromo(code: string): Promise<{ code: string; discount_percent: number }> {
  return request(`/api/promo/${encodeURIComponent(code)}`);
}

export function fetchMe(): Promise<{ name?: string; phone?: string; promo_code?: string; discount_percent?: number }> {
  return request('/api/me');
}

export function fetchFreshToday(): Promise<{ items?: string }> {
  return request('/api/fresh-today');
}
