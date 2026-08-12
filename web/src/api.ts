import type { DeliveryType, Order, Product, ProductCard } from './types';
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
  delivery_type: DeliveryType;
  metro_station: string; // при доставке до метро
  delivery_address: string; // при доставке по адресу
  delivery_date: string;
  delivery_time: string; // может быть пустым: время необязательно
  comment: string;
  card_text: string;
  is_anonymous: boolean;
  promo_code: string;
}

export function createOrder(payload: OrderPayload): Promise<Order> {
  return request('/api/orders', { method: 'POST', body: JSON.stringify(payload) });
}

/** История заказов текущего клиента (по initData Telegram). */
export function fetchMyOrders(): Promise<Order[]> {
  return request('/api/orders');
}

// Список станций метро статичен (живёт в коде бэкенда) — держим его в памяти
// на всё время сессии, чтобы поиск в чекауте работал без сети.
let stationsCache: Promise<string[]> | null = null;

export function fetchMetroStations(): Promise<string[]> {
  if (!stationsCache) {
    stationsCache = request<string[]>('/api/metro-stations');
    stationsCache.catch(() => {
      stationsCache = null;
    });
  }
  return stationsCache;
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
