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

/**
 * Трекинг запуска Mini App: сервер парсит startapp-параметр из initData
 * и запоминает first-touch источник клиента (для /stats в админ-боте).
 */
export function trackLaunch(): Promise<Record<string, never>> {
  return request('/api/launch', { method: 'POST' });
}

export interface SubscriptionBonus {
  enabled: boolean;
  claimed: boolean;
  code?: string;
  discount_percent?: number;
  channel_url?: string;
}

export function fetchSubscriptionBonus(): Promise<SubscriptionBonus> {
  return request('/api/subscription-bonus');
}

/** Проверяет подписку на канал через бота и выдаёт одноразовый промокод. */
export function claimSubscriptionBonus(): Promise<SubscriptionBonus> {
  return request('/api/subscription-bonus', { method: 'POST' });
}
