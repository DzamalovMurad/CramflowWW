/**
 * Интеграция с Telegram WebApp SDK: инициализация, тёмная тема, haptics, BackButton.
 * Все вызовы обёрнуты в проверки — приложение работает и в обычном браузере.
 */

type TelegramWebApp = {
  initData: string;
  initDataUnsafe?: { start_param?: string };
  colorScheme: 'light' | 'dark';
  themeParams: Record<string, string>;
  ready: () => void;
  expand: () => void;
  onEvent: (event: string, cb: () => void) => void;
  offEvent: (event: string, cb: () => void) => void;
  BackButton: { show: () => void; hide: () => void; onClick: (cb: () => void) => void; offClick: (cb: () => void) => void };
  HapticFeedback?: { impactOccurred: (style: string) => void; notificationOccurred: (type: string) => void };
  setHeaderColor?: (color: string) => void;
  setBackgroundColor?: (color: string) => void;
};

export function tg(): TelegramWebApp | undefined {
  return (window as any).Telegram?.WebApp;
}

export function initDataHeader(): Record<string, string> {
  const data = tg()?.initData;
  return data ? { 'X-Telegram-Init-Data': data } : {};
}

function applyScheme() {
  // Тему применяет модуль theme.ts: выбор пользователя важнее темы Telegram.
  import('./theme').then(({ currentTheme, applyTheme }) => applyTheme(currentTheme()));
}

export function initTelegram() {
  const app = tg();
  applyScheme();
  if (!app) return;
  app.ready();
  app.expand();
  app.onEvent('themeChanged', applyScheme);
  capturePromoStartParam();
}

// --- Deep-link промокод: t.me/bot/app?startapp=promo_<CODE> ---

const PROMO_KEY = 'flowix_pending_promo';

// Запоминаем код из start_param: применится автоматически в checkout.
function capturePromoStartParam() {
  const param = tg()?.initDataUnsafe?.start_param;
  if (param?.startsWith('promo_')) {
    const code = param.slice('promo_'.length).toUpperCase();
    if (code) localStorage.setItem(PROMO_KEY, code);
  }
}

/** Промокод из deep-link, ждущий применения в checkout (null — нет). */
export function pendingPromo(): string | null {
  return localStorage.getItem(PROMO_KEY);
}

/** Сбрасываем сохранённый deep-link-код (после оформления заказа). */
export function clearPendingPromo() {
  localStorage.removeItem(PROMO_KEY);
}

export function haptic(style: 'light' | 'medium' | 'success' = 'light') {
  const h = tg()?.HapticFeedback;
  if (!h) return;
  if (style === 'success') h.notificationOccurred('success');
  else h.impactOccurred(style);
}
