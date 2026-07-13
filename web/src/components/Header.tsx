import { Link, useNavigate } from 'react-router-dom';
import { useCart } from '../cart';

/** Шапка: логотип + корзина с бейджем. showBack — стрелка для браузера без Telegram. */
export default function Header({ title, showBack }: { title?: string; showBack?: boolean }) {
  const { count } = useCart();
  const navigate = useNavigate();

  return (
    <header className="sticky top-0 z-20 flex h-14 items-center justify-between border-b border-line bg-page/95 px-4 backdrop-blur">
      <div className="flex items-center gap-2">
        {showBack && (
          <button
            onClick={() => navigate(-1)}
            aria-label="Назад"
            className="-ml-2 flex h-9 w-9 items-center justify-center rounded-full text-xl active:bg-line"
          >
            ←
          </button>
        )}
        {title ? (
          <span className="text-lg font-semibold">{title}</span>
        ) : (
          <Link to="/" className="text-lg font-bold tracking-tight">
            Cram<span className="text-accent-2">Flow</span>
          </Link>
        )}
      </div>
      <Link
        to="/cart"
        aria-label="Корзина"
        className="relative flex h-9 w-9 items-center justify-center rounded-full text-xl active:bg-line"
      >
        🛒
        {count > 0 && (
          <span className="absolute -right-1 -top-1 flex h-5 min-w-5 items-center justify-center rounded-full bg-accent px-1 text-xs font-bold text-[#111111]">
            {count}
          </span>
        )}
      </Link>
    </header>
  );
}
