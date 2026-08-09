/**
 * Тонкая обёртка над Telegram WebApp SDK: инициализация, haptics, BackButton.
 * Все вызовы защищены проверками — приложение работает и в обычном браузере.
 *
 * Тему модуль не трогает: этим занимается theme.ts, иначе получается
 * циклическая зависимость и лишний динамический чанк в сборке.
 */

type TelegramWebApp = {
  initData: string;
  colorScheme: 'light' | 'dark';
  themeParams: Record<string, string>;
  ready: () => void;
  expand: () => void;
  onEvent: (event: string, cb: () => void) => void;
  offEvent: (event: string, cb: () => void) => void;
  BackButton: {
    show: () => void;
    hide: () => void;
    onClick: (cb: () => void) => void;
    offClick: (cb: () => void) => void;
  };
  HapticFeedback?: {
    impactOccurred: (style: string) => void;
    notificationOccurred: (type: string) => void;
  };
  setHeaderColor?: (color: string) => void;
  setBackgroundColor?: (color: string) => void;
};

export function tg(): TelegramWebApp | undefined {
  return (window as unknown as { Telegram?: { WebApp?: TelegramWebApp } }).Telegram?.WebApp;
}

/** Заголовок с подписанной initData — сервер проверяет её на каждом запросе. */
export function initDataHeader(): Record<string, string> {
  const data = tg()?.initData;
  return data ? { 'X-Telegram-Init-Data': data } : {};
}

/** Разворачивает Mini App на всю высоту и сообщает Telegram о готовности. */
export function initTelegram() {
  const app = tg();
  if (!app) return;
  app.ready();
  app.expand();
}

/** Подписка на смену темы в Telegram. */
export function onThemeChanged(cb: () => void) {
  tg()?.onEvent('themeChanged', cb);
}

export function haptic(style: 'light' | 'medium' | 'success' = 'light') {
  const h = tg()?.HapticFeedback;
  if (!h) return;
  if (style === 'success') h.notificationOccurred('success');
  else h.impactOccurred(style);
}
