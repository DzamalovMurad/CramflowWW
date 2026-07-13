import { Link, useNavigate } from 'react-router-dom';
import { useCart } from '../cart';
import { content } from '../content';
import { haptic } from '../telegram';
import { toggleTheme } from '../theme';
import { IconArrowLeft, IconBag, IconMoon, IconSun } from './icons';

/** Шапка: строчный wordmark + переключатель темы + корзина. */
export default function Header({ title, showBack }: { title?: string; showBack?: boolean }) {
  const { count } = useCart();
  const navigate = useNavigate();

  return (
    <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-line bg-page/95 px-4 backdrop-blur">
      <div className="flex items-center gap-1">
        {showBack && (
          <button
            onClick={() => navigate(-1)}
            aria-label="назад"
            className="-ml-2 flex h-10 w-10 items-center justify-center rounded-full active:bg-tile"
          >
            <IconArrowLeft />
          </button>
        )}
        <Link to="/" className="text-[19px] font-bold lowercase tracking-tight">
          {content.brand}
        </Link>
        {title && (
          <span className="ml-2 border-l border-line pl-3 text-sm lowercase text-muted">{title}</span>
        )}
      </div>
      <div className="flex items-center">
      <button
        type="button"
        aria-label="сменить тему"
        onClick={() => {
          haptic('light');
          toggleTheme();
        }}
        className="theme-toggle flex h-10 w-10 items-center justify-center rounded-full active:bg-tile"
      >
        <span className="icon-sun"><IconSun /></span>
        <span className="icon-moon"><IconMoon /></span>
      </button>
      <Link
        to="/cart"
        aria-label="корзина"
        className="relative -mr-1 flex h-10 w-10 items-center justify-center rounded-full active:bg-tile"
      >
        <IconBag />
        {count > 0 && (
          <span className="absolute right-0 top-0 flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-accent px-1 text-[11px] font-bold text-on-accent">
            {count}
          </span>
        )}
      </Link>
      </div>
    </header>
  );
}
