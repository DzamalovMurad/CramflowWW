import { Suspense, lazy, useEffect, useState } from 'react';
import { Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import Home from './pages/Home';
import TabBar from './components/TabBar';
import AppLoader from './components/AppLoader';
import ErrorBoundary from './components/ErrorBoundary';
import { warmUp } from './warmup';
import { tg } from './telegram';

// Главная нужна на первом экране — она в основном бандле.
// Остальные экраны подгружаются при переходе: на первую отрисовку не должны
// влиять форма оформления заказа и экран профиля.
const Catalog = lazy(() => import('./pages/Catalog'));
const ProductPage = lazy(() => import('./pages/Product'));
const Cart = lazy(() => import('./pages/Cart'));
const Checkout = lazy(() => import('./pages/Checkout'));
const Confirmation = lazy(() => import('./pages/Confirmation'));
const Profile = lazy(() => import('./pages/Profile'));

export default function App() {
  const location = useLocation();
  const navigate = useNavigate();

  // Заставка держится ровно столько, сколько грузится каталог, и не дольше
  // трёх секунд. Раньше здесь была искусственная задержка в 1.5 секунды —
  // это чистая потеря клиентов на входе.
  const [booted, setBooted] = useState(false);
  useEffect(() => {
    let alive = true;
    const finish = () => {
      if (alive) setBooted(true);
    };
    const cap = setTimeout(finish, 3000);
    warmUp().finally(() => {
      clearTimeout(cap);
      finish();
    });
    return () => {
      alive = false;
      clearTimeout(cap);
    };
  }, []);

  // Кнопка «Назад» Telegram на всех страницах, кроме главной.
  useEffect(() => {
    const app = tg();
    if (!app) return;
    const goBack = () => navigate(-1);
    if (location.pathname === '/') {
      app.BackButton.hide();
      return;
    }
    app.BackButton.show();
    app.BackButton.onClick(goBack);
    return () => {
      app.BackButton.offClick(goBack);
      app.BackButton.hide();
    };
  }, [location.pathname, navigate]);

  // Скроллим к началу при переходах.
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [location.pathname]);

  // Нижнее меню — на просмотровых экранах. Не в корзине (там свой нижний бар),
  // не в оформлении и не в карточке товара.
  const showTabBar = ['/', '/catalog', '/profile'].includes(location.pathname);

  return (
    <div className="mx-auto min-h-screen max-w-md font-sans text-ink">
      <div className="ambient" aria-hidden />
      <div className="grain" aria-hidden />
      <ErrorBoundary>
        <Suspense fallback={<RouteFallback />}>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/catalog" element={<Catalog />} />
            <Route path="/product/:id" element={<ProductPage />} />
            <Route path="/cart" element={<Cart />} />
            <Route path="/checkout" element={<Checkout />} />
            <Route path="/confirmation/:id" element={<Confirmation />} />
            <Route path="/profile" element={<Profile />} />
            {/* Незнакомый адрес не должен давать пустой экран. */}
            <Route path="*" element={<Home />} />
          </Routes>
        </Suspense>
      </ErrorBoundary>
      {showTabBar && <TabBar />}
      <AppLoader done={booted} />
    </div>
  );
}

/** Скелетон на время загрузки чанка страницы — вместо пустого экрана. */
function RouteFallback() {
  return (
    <div className="p-4 pt-6">
      <div className="h-8 w-1/2 animate-pulse rounded bg-tile" />
      <div className="mt-6 grid grid-cols-2 gap-x-3 gap-y-6">
        {[0, 1, 2, 3].map((i) => (
          <div key={i}>
            <div className="aspect-[4/5] animate-pulse rounded-card bg-tile" />
            <div className="mt-2.5 h-3.5 w-2/3 animate-pulse rounded bg-tile" />
          </div>
        ))}
      </div>
    </div>
  );
}
