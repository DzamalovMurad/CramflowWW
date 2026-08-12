/**
 * Тема (день/ночь): выбор пользователя хранится в localStorage и важнее
 * темы Telegram; без сохранённого выбора берём схему Telegram или системную.
 */
import { tg } from './telegram';

export type Theme = 'light' | 'dark';

const KEY = 'cf-theme';

/**
 * Цвета шапки и фона, которые приложение отдаёт Telegram. Обязаны совпадать
 * с токеном --c-bg в styles/index.css: иначе шапка клиента и фон Mini App
 * расходятся на пару тонов и по краю экрана видна чужая полоса.
 * Раньше здесь лежали цвета от прошлой палитры — расхождение и было.
 */
const TG_COLORS: Record<Theme, string> = { light: '#EDECEA', dark: '#131311' };

export function currentTheme(): Theme {
  const saved = localStorage.getItem(KEY);
  if (saved === 'light' || saved === 'dark') return saved;
  if (tg()) return tg()!.colorScheme === 'dark' ? 'dark' : 'light';
  return matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  tg()?.setHeaderColor?.(TG_COLORS[theme]);
  tg()?.setBackgroundColor?.(TG_COLORS[theme]);
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
