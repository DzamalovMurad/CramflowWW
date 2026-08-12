/**
 * Тема (день/ночь): выбор пользователя хранится в localStorage и важнее
 * темы Telegram; без сохранённого выбора берём схему Telegram или системную.
 */
import { tg } from './telegram';

export type Theme = 'light' | 'dark';

const KEY = 'cf-theme';

/**
 * Системная шапка Telegram — near-black в обеих темах: тот же цвет, что
 * у шапки приложения и плашки логотипа (--badge-ink). Верх экрана читается
 * одной тёмной полосой; при светлой системной шапке она разрывалась
 * на светлую полосу клиента и тёмную марку под ней.
 */
const TG_HEADER = '#1A1A18';

/**
 * Фон под контентом отдаём отдельно и по теме: он обязан совпадать
 * с токеном --c-bg, иначе при оттягивании страницы из-под неё выглядывает
 * чужой цвет.
 */
const TG_BG: Record<Theme, string> = { light: '#EDECEA', dark: '#131311' };

export function currentTheme(): Theme {
  const saved = localStorage.getItem(KEY);
  if (saved === 'light' || saved === 'dark') return saved;
  if (tg()) return tg()!.colorScheme === 'dark' ? 'dark' : 'light';
  return matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  tg()?.setHeaderColor?.(TG_HEADER);
  tg()?.setBackgroundColor?.(TG_BG[theme]);
}

/** Переключение с плавным перетеканием цветов (класс theme-anim на время перехода). */
export function toggleTheme(): Theme {
  const next: Theme = currentTheme() === 'dark' ? 'light' : 'dark';
  localStorage.setItem(KEY, next);
  const root = document.documentElement;
  root.classList.add('theme-anim');
  applyTheme(next);
  window.setTimeout(() => root.classList.remove('theme-anim'), 600);
  return next;
}
