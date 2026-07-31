import type { AppConfig, Order, Product, ProductCard } from './types';
import { initDataHeader, rememberBotUsername } from './telegram';

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
    // Конфиг греем заодно: имя бота понадобится экрану ошибки, а он не ждёт.
    fetchConfig().catch(() => {});
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

/** Подборка хитов: пустая корзина ведёт сюда, а не в никуда. */
export function fetchHits(limit = 5): Promise<ProductCard[]> {
  return request(`/api/products/hits?limit=${limit}`);
}

/**
 * Конфиг развёрнутого сервиса: имя бота (нужно экрану ошибки) и признак
 * fallback-режима. Кэшируем — значение меняется только при редеплое.
 */
let configCache: Promise<AppConfig> | null = null;

export function fetchConfig(): Promise<AppConfig> {
  if (!configCache) {
    configCache = request<AppConfig>('/api/config')
      .then((cfg) => {
        rememberBotUsername(cfg.bot_username ?? '');
        return cfg;
      })
      .catch((e) => {
        configCache = null;
        throw e;
      });
  }
  return configCache;
}
