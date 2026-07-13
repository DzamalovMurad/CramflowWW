/**
 * Интеграция с Telegram WebApp SDK: инициализация, тёмная тема, haptics, BackButton.
 * Все вызовы обёрнуты в проверки — приложение работает и в обычном браузере.
 */

type TelegramWebApp = {
  initData: string;
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
  const dark = tg()?.colorScheme === 'dark';
  document.documentElement.dataset.theme = dark ? 'dark' : 'light';
  tg()?.setHeaderColor?.(dark ? '#101010' : '#FFFFFF');
  tg()?.setBackgroundColor?.(dark ? '#101010' : '#FFFFFF');
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
