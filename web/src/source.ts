/**
 * Канал привлечения (utm-метка). Берётся один раз — при первом заходе —
 * и живёт до заказа: человек может открыть приложение по рекламной ссылке,
 * а оформить заказ через день с главного экрана, и заказ всё равно должен
 * достаться правильному каналу.
 *
 * Источники, в порядке приоритета:
 *   1. start_param Telegram Mini App — t.me/bot/app?startapp=instagram
 *   2. ?src= / ?utm_source= в адресе страницы
 */

const STORAGE_KEY = 'cramflow_source_v1';

function fromTelegram(): string {
  const params = (window as any).Telegram?.WebApp?.initDataUnsafe;
  return params?.start_param || '';
}

function fromQuery(): string {
  const q = new URLSearchParams(window.location.search);
  return q.get('src') || q.get('utm_source') || '';
}

/** Запоминает метку канала при первом заходе. Вызывается один раз при старте. */
export function captureSource(): void {
  try {
    if (localStorage.getItem(STORAGE_KEY)) return; // первый канал важнее последнего
    const src = fromTelegram() || fromQuery();
    if (src) localStorage.setItem(STORAGE_KEY, src.slice(0, 32));
  } catch {
    // Приватный режим блокирует localStorage — канал просто будет «direct».
  }
}

/** Метка канала для заказа. Сервер её нормализует, пустая означает «напрямую». */
export function currentSource(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) || '';
  } catch {
    return '';
  }
}
