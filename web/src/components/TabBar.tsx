import { Link, useLocation } from 'react-router-dom';
import { useCart } from '../cart';
import { haptic } from '../telegram';
import { IconGrid, IconBag, IconUser } from './icons';

type Tab = { to: string; label: string; Icon: (p: { size?: number }) => JSX.Element; cart?: boolean };

const tabs: Tab[] = [
  { to: '/', label: 'меню', Icon: IconGrid },
  { to: '/cart', label: 'корзина', Icon: IconBag, cart: true },
  { to: '/profile', label: 'профиль', Icon: IconUser },
];

/** Нижнее меню-бар. Активная вкладка — чартрезовая плашка под иконкой. */
export default function TabBar() {
  const { pathname } = useLocation();
  const { count } = useCart();

  return (
    <nav className="tabbar tabbar-solid">
      <div className="flex items-stretch">
        {tabs.map(({ to, label, Icon, cart }) => {
          const active = to === '/' ? pathname === '/' : pathname.startsWith(to);
          return (
            <Link
              key={to}
              to={to}
              onClick={() => haptic('light')}
              className="relative flex flex-1 flex-col items-center gap-1.5 py-3"
            >
              <span
                className={`relative flex items-center justify-center transition-colors ${
                  active ? 'tab-pill' : 'h-[30px] w-[46px] text-muted'
                }`}
                {...(cart ? { 'data-cart-icon': true } : {})}
              >
                <Icon size={24} />
                {cart && count > 0 && (
                  <span className="absolute -right-1 -top-1 flex h-[17px] min-w-[17px] items-center justify-center rounded-full border-2 border-surface bg-ink nums px-1 text-[10px] font-bold text-page">
                    {count}
                  </span>
                )}
              </span>
              <span
                className={`text-[11px] lowercase transition-colors ${
                  active ? 'font-bold text-ink' : 'font-medium text-muted'
                }`}
              >
                {label}
              </span>
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
