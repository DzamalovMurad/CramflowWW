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

/**
 * startapp-параметр запуска Mini App (t.me/bot?startapp=...):
 * product_<id> открывает карточку товара, src_<tag> — метка источника.
 * Вне Telegram параметр приходит в query как tgWebAppStartParam.
 */
export function startParam(): string {
  return (
    tg()?.initDataUnsafe?.start_param ??
    new URLSearchParams(window.location.search).get('tgWebAppStartParam') ??
    ''
  );
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
}

export function haptic(style: 'light' | 'medium' | 'success' = 'light') {
  const h = tg()?.HapticFeedback;
  if (!h) return;
  if (style === 'success') h.notificationOccurred('success');
  else h.impactOccurred(style);
}
