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
  openTelegramLink?: (url: string) => void;
  close?: () => void;
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
 * product_<id> открывает карточку товара, order_<id> — заказ из пуша
 * о смене статуса, src_<tag> — метка источника.
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

export function haptic(style: 'light' | 'medium' | 'success' | 'warning' | 'error' = 'light') {
  const h = tg()?.HapticFeedback;
  if (!h) return;
  if (style === 'success' || style === 'warning' || style === 'error') h.notificationOccurred(style);
  else h.impactOccurred(style);
}

/** Номер заказа из пуша о статусе (0, если приложение открыли обычным способом). */
export function orderFromStartParam(): number {
  const match = /^order[_-](\d+)$/.exec(startParam());
  return match ? Number(match[1]) : 0;
}

/**
 * openBotChat — выход в диалог с ботом. Это аварийный путь: им пользуются
 * экран ошибки и режим, когда Mini App недоступен, поэтому имя бота берём
 * из /api/config, а при неудаче просто закрываем приложение — клиент окажется
 * в том же чате, откуда пришёл.
 */
export function openBotChat(username?: string) {
  const name = (username ?? cachedBotUsername).replace(/^@/, '');
  const app = tg();
  if (name) {
    const link = `https://t.me/${name}`;
    if (app?.openTelegramLink) {
      app.openTelegramLink(link);
      return;
    }
    window.open(link, '_blank');
    return;
  }
  app?.close?.();
}

// Имя бота кэшируем при загрузке конфига: экран ошибки не может ждать запрос.
let cachedBotUsername = '';

export function rememberBotUsername(name: string) {
  cachedBotUsername = name ?? '';
}
