import { Link, useNavigate } from 'react-router-dom';
import { content } from '../content';
import { haptic } from '../telegram';
import { toggleTheme } from '../theme';
import { IconArrowLeft, IconMoon, IconSun } from './icons';

/** Шапка: wordmark + переключатель темы. Корзина живёт в нижнем меню. */
export default function Header({ title, showBack }: { title?: string; showBack?: boolean }) {
  const navigate = useNavigate();

  return (
    <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-line bg-page/80 px-4 backdrop-blur-xl">
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
        <Link to="/" className="text-[19px] font-extrabold lowercase tracking-tight">
          {content.brand}
          <span className="text-accent-2">.</span>
        </Link>
        {title && (
          <span className="ml-2 border-l border-line pl-3 text-sm lowercase text-muted">{title}</span>
        )}
      </div>

      <button
        type="button"
        aria-label="сменить тему"
        onClick={() => {
          haptic('light');
          toggleTheme();
        }}
        className="theme-toggle flex h-9 w-9 items-center justify-center rounded-full border border-line bg-tile/60 text-ink backdrop-blur active:scale-90"
      >
        <span className="icon-sun"><IconSun size={18} /></span>
        <span className="icon-moon"><IconMoon size={18} /></span>
      </button>
    </header>
  );
}
