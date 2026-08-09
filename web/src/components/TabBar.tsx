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

/** Нижнее меню-бар (стиль Bunch). Активная вкладка — неоновая. */
export default function TabBar() {
  const { pathname } = useLocation();
  const { count } = useCart();

  return (
    <nav className="tabbar">
      <div className="flex items-stretch">
        {tabs.map(({ to, label, Icon, cart }) => {
          const active = to === '/' ? pathname === '/' : pathname.startsWith(to);
          return (
            <Link
              key={to}
              to={to}
              onClick={() => haptic('light')}
              className="relative flex flex-1 flex-col items-center gap-1 py-2.5"
            >
              <span
                className={`relative transition-colors ${active ? 'text-ink' : 'text-muted'}`}
                {...(cart ? { 'data-cart-icon': true } : {})}
              >
                <Icon size={23} />
                {cart && count > 0 && (
                  <span className="absolute -right-2 -top-1.5 flex h-[16px] min-w-[16px] items-center justify-center rounded-full bg-accent px-1 text-[10px] font-extrabold text-on-accent">
                    {count}
                  </span>
                )}
              </span>
              <span
                className={`text-[10px] lowercase transition-colors ${
                  active ? 'font-bold text-ink' : 'text-muted'
                }`}
              >
                {label}
              </span>
              {active && (
                <span className="absolute bottom-0 h-[3px] w-7 rounded-full bg-accent" />
              )}
            </Link>
          );
        })}
      </div>
    </nav>
  );
}
