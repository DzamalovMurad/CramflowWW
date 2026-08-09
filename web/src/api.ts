import type { Order, Product, ProductCard, Promo, ShopConfig } from './types';
import { initDataHeader } from './telegram';
import { content } from './content';

/** Ошибка API: несёт текст для пользователя и, если есть, машинные подробности. */
export class ApiError extends Error {
  readonly status: number;
  /** Позиции корзины, которых больше нет (сервер присылает их при 400). */
  readonly unavailableVariantIds: number[];
  /** Сеть недоступна — имеет смысл предложить повтор. */
  readonly offline: boolean;

  constructor(
    message: string,
    status: number,
    opts: { unavailable?: number[]; offline?: boolean } = {},
  ) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.unavailableVariantIds = opts.unavailable ?? [];
    this.offline = opts.offline ?? false;
  }
}

/** Потолок ожидания ответа: замерший запрос не должен вешать интерфейс. */
const REQUEST_TIMEOUT_MS = 20_000;

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), REQUEST_TIMEOUT_MS);

  let res: Response;
  try {
    res = await fetch(path, {
      ...init,
      signal: controller.signal,
      headers: {
        'Content-Type': 'application/json',
        ...initDataHeader(),
        ...init?.headers,
      },
    });
  } catch {
    // Сеть пропала или запрос оборвался по таймауту.
    throw new ApiError(content.common.offline, 0, { offline: true });
  } finally {
    clearTimeout(timer);
  }

  const body = (await res.json().catch(() => ({}))) as Record<string, unknown>;
  if (!res.ok) {
    throw new ApiError(
      typeof body.error === 'string' ? body.error : 'что-то пошло не так, попробуйте ещё раз',
      res.status,
      {
        unavailable: Array.isArray(body.unavailable_variant_ids)
          ? (body.unavailable_variant_ids as number[])
          : [],
      },
    );
  }
  return body as T;
}

export function fetchProducts(
  category: string,
  filter: string,
  search = '',
): Promise<ProductCard[]> {
  const params = new URLSearchParams();
  if (category) params.set('category', category);
  if (filter) params.set('filter', filter);
  if (search) params.set('q', search);
  const qs = params.toString();
  return request(`/api/products${qs ? `?${qs}` : ''}`);
}

/**
 * Кэш полного каталога на 60 секунд: главная и каталог открываются мгновенно,
 * а правки из админ-бота всё равно доезжают меньше чем за минуту.
 */
let allCache: { at: number; data: Promise<ProductCard[]> } | null = null;

export function fetchAllProducts(): Promise<ProductCard[]> {
  if (allCache && Date.now() - allCache.at < 60_000) return allCache.data;
  const data = fetchProducts('', '');
  allCache = { at: Date.now(), data };
  data.catch(() => {
    allCache = null; // неудачный запрос кэшировать нельзя
  });
  return data;
}

/** Сброс кэша каталога — после оформления заказа остатки могли измениться. */
export function invalidateCatalog() {
  allCache = null;
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
  recipient_name: string;
  recipient_phone: string;
  comment: string;
  card_text: string;
  is_anonymous: boolean;
  promo_code: string;
}

/**
 * createOrder передаёт ключ идемпотентности: повторная отправка той же формы
 * (двойной тап, ретрай после обрыва сети) вернёт уже созданный заказ,
 * а не создаст второй.
 */
export function createOrder(payload: OrderPayload, idempotencyKey: string): Promise<Order> {
  return request('/api/orders', {
    method: 'POST',
    body: JSON.stringify(payload),
    headers: { 'Idempotency-Key': idempotencyKey },
  });
}

export function fetchOrder(id: number | string): Promise<Order> {
  return request(`/api/orders/${id}`);
}

export function fetchMyOrders(): Promise<Order[]> {
  return request('/api/my/orders');
}

/**
 * checkPromo передаёт сумму корзины: сервер сам проверит минимальную сумму
 * заказа и вернёт готовое правило скидки — процент или рубли.
 */
export function checkPromo(code: string, subtotal?: number): Promise<Promo> {
  const query = subtotal && subtotal > 0 ? `?subtotal=${subtotal}` : '';
  return request(`/api/promo/${encodeURIComponent(code)}${query}`);
}

export function fetchMe(): Promise<{
  name?: string;
  phone?: string;
  telegram_name?: string;
  promo_code?: string;
  discount_percent?: number;
  discount_type?: 'percent' | 'fixed';
  discount_value?: number;
  min_order_amount?: number;
}> {
  return request('/api/me');
}

export function fetchFreshToday(): Promise<{ items?: string }> {
  return request('/api/fresh-today');
}

/**
 * Правила доставки берём с сервера, а не хардкодим: форма не должна
 * предлагать варианты, которые сервер всё равно отклонит.
 */
export function fetchConfig(): Promise<ShopConfig> {
  return request('/api/config');
}

/** Уникальный ключ попытки оформления заказа. */
export function newIdempotencyKey(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}
