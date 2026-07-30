import type { AddonCard, Order, Product, ProductCard, PromoInfo, RepeatItem, SlotDay } from './types';
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

// Кэш полного каталога: греется лоадером при старте, TTL 60с,
// чтобы правки из админ-бота не «зависали» в приложении.
let allCache: { at: number; data: Promise<ProductCard[]> } | null = null;

export function fetchAllProducts(): Promise<ProductCard[]> {
  if (allCache && Date.now() - allCache.at < 60_000) return allCache.data;
  const data = fetchProducts('', '');
  allCache = { at: Date.now(), data };
  data.catch(() => {
    allCache = null;
  });
  return data;
}

/** Прогрев для стартового лоадера: каталог + первые фото букетов в кэш браузера. */
export async function warmUp(): Promise<void> {
  try {
    const list = await fetchAllProducts();
    await Promise.all(
      list.slice(0, 6).map((p) =>
        p.image
          ? new Promise<void>((res) => {
              const im = new Image();
              im.onload = im.onerror = () => res();
              im.src = p.image;
            })
          : Promise.resolve(),
      ),
    );
  } catch {
    // Лоадер не должен блокировать вход при ошибке сети.
  }
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
  delivery_time: string; // слот «10:00-12:00» … «20:00-22:00»
  comment: string;
  card_text: string;
  is_anonymous: boolean;
  recipient_name: string;
  recipient_phone: string;
  address_by_recipient: boolean;
  promo_code: string;
  idempotency_key: string;
}

export function createOrder(payload: OrderPayload): Promise<Order> {
  return request('/api/orders', { method: 'POST', body: JSON.stringify(payload) });
}

/** Проверка промокода по корзине: сервер считает все правила и итоговую скидку. */
export function checkPromo(code: string, items: { variant_id: number; quantity: number }[]): Promise<PromoInfo> {
  return request('/api/promo/check', { method: 'POST', body: JSON.stringify({ code, items }) });
}

export function fetchSlots(): Promise<{ days: SlotDay[] }> {
  return request('/api/delivery-slots');
}

export function fetchAddons(): Promise<AddonCard[]> {
  return request('/api/addons');
}

export function fetchMyOrders(): Promise<Order[]> {
  return request('/api/my-orders');
}

export function fetchRepeatOrder(id: number): Promise<{ items: RepeatItem[] }> {
  return request(`/api/orders/${id}/repeat`);
}

/** uuid для идемпотентного создания заказа (fallback для старых WebView). */
export function uuid(): string {
  if (typeof crypto !== 'undefined' && crypto.randomUUID) return crypto.randomUUID();
  const b = new Uint8Array(16);
  crypto.getRandomValues(b);
  b[6] = (b[6] & 0x0f) | 0x40;
  b[8] = (b[8] & 0x3f) | 0x80;
  const h = [...b].map((x) => x.toString(16).padStart(2, '0')).join('');
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

export function fetchMe(): Promise<{ name?: string; phone?: string; promo_code?: string; promo_label?: string }> {
  return request('/api/me');
}

export function fetchFreshToday(): Promise<{ items?: string }> {
  return request('/api/fresh-today');
}
