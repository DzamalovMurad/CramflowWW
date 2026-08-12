import { Link, useNavigate } from 'react-router-dom';
import { content } from '../content';
import { haptic } from '../telegram';
import { toggleTheme } from '../theme';
import { IconArrowLeft, IconMoon, IconSun, IconSearch, IconClose } from './icons';

interface SearchProps {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
}

/** Шапка меню (главная): крупный центрированный wordmark + строка поиска.
    Не sticky — при скролле сверху остаются только фильтры (как у Bunch). */
export function MenuHeader() {
  return (
    <header className="app-header px-4 pb-3 pt-2.5">
      <div className="relative flex h-10 items-center justify-center">
        <Link
          to="/"
          className="flex min-h-[44px] items-center px-2"
        >
          <span className="brand text-[22px]">
            <span className="brand-in">
              {content.brand}
              <span className="brand-dot">.</span>
            </span>
          </span>
        </Link>
        <button
          type="button"
          aria-label="сменить тему"
          onClick={() => {
            haptic('light');
            toggleTheme();
          }}
          className="theme-toggle header-icon absolute right-0 flex h-11 w-11 items-center justify-center rounded-full border active:scale-90"
        >
          <span className="icon-sun"><IconSun size={18} /></span>
          <span className="icon-moon"><IconMoon size={18} /></span>
        </button>
      </div>
      <Link
        to="/catalog"
        className="mt-2 flex min-h-[48px] items-center justify-between rounded-full border border-line bg-surface px-4 shadow-card"
      >
        <span className="text-[14px] lowercase text-muted">{content.catalog.searchPlaceholder}</span>
        <span className="text-muted"><IconSearch size={17} /></span>
      </Link>
    </header>
  );
}

/** Шапка: wordmark + (поиск ↔ заголовок) + переключатель темы. Корзина — в нижнем меню. */
export default function Header({
  title,
  showBack,
  search,
}: {
  title?: string;
  showBack?: boolean;
  search?: SearchProps;
}) {
  const navigate = useNavigate();

  return (
    <header className="app-header sticky top-0 z-20 flex h-14 items-center gap-2 px-4">
      <div className="flex flex-shrink-0 items-center gap-1">
        {showBack && (
          <button
            onClick={() => navigate(-1)}
            aria-label="назад"
            className="header-icon -ml-2 flex h-11 w-11 items-center justify-center rounded-full !border-transparent !bg-transparent active:opacity-60"
          >
            <IconArrowLeft />
          </button>
        )}
        <Link
          to="/"
          className="flex min-h-[44px] items-center"
        >
          <span className="brand text-[17px]">
            <span className="brand-in">
              {content.brand}
              <span className="brand-dot">.</span>
            </span>
          </span>
        </Link>
        {title && !search && (
          <span className="header-title ml-2 border-l pl-3 text-sm lowercase">{title}</span>
        )}
      </div>

      {search && (
        <div className="flex min-w-0 flex-1 items-center gap-2 rounded-full bg-surface px-3 py-2">
          <span className="flex-shrink-0 text-muted">
            <IconSearch size={17} />
          </span>
          <input
            value={search.value}
            onChange={(e) => search.onChange(e.target.value)}
            placeholder={search.placeholder ?? content.catalog.searchPlaceholder}
            className="search-input min-w-0 flex-1 bg-transparent text-[14px] text-ink placeholder:text-muted"
          />
          {search.value && (
            <button
              type="button"
              aria-label="очистить"
              onClick={() => {
                haptic('light');
                search.onChange('');
              }}
              className="-mr-1 flex h-11 w-11 flex-shrink-0 items-center justify-center text-muted active:opacity-50"
            >
              <IconClose size={15} />
            </button>
          )}
        </div>
      )}

      <button
        type="button"
        aria-label="сменить тему"
        onClick={() => {
          haptic('light');
          toggleTheme();
        }}
        className="theme-toggle header-icon ml-auto flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full border active:scale-90"
      >
        <span className="icon-sun"><IconSun size={18} /></span>
        <span className="icon-moon"><IconMoon size={18} /></span>
      </button>
    </header>
  );
}
