import { useEffect, useRef, useState } from 'react';
import { Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import Home from './pages/Home';
import Catalog from './pages/Catalog';
import ProductPage from './pages/Product';
import Cart from './pages/Cart';
import Checkout from './pages/Checkout';
import Confirmation from './pages/Confirmation';
import Profile from './pages/Profile';
import TabBar from './components/TabBar';
import AppLoader from './components/AppLoader';
import { trackLaunch, warmUp } from './api';
import { haptic, orderFromStartParam, startParam, tg } from './telegram';

export default function App() {
  const location = useLocation();
  const navigate = useNavigate();

  // startapp-параметр (t.me/bot?startapp=...): трекинг источника на сервере
  // и прямое открытие карточки товара из поста в канале. Один раз за запуск.
  const startHandled = useRef(false);
  useEffect(() => {
    if (startHandled.current) return;
    startHandled.current = true;
    trackLaunch().catch(() => {});
    const match = /^product_(\d+)$/.exec(startParam());
    if (match) navigate(`/product/${match[1]}`, { replace: true });
  }, [navigate]);

  // Стартовая заставка: минимум 1.5с, максимум 3с; за это время
  // warmUp() кладёт каталог и первые фото букетов в кэш.
  const [booted, setBooted] = useState(false);
  useEffect(() => {
    let alive = true;
    const min = new Promise((r) => setTimeout(r, 1500));
    const cap = new Promise((r) => setTimeout(r, 3000));
    Promise.race([Promise.all([warmUp(), min]), cap]).then(() => alive && setBooted(true));
    return () => {
      alive = false;
    };
  }, []);

  // Пуш о смене статуса открывает Mini App с параметром order_<id>:
  // сразу везём клиента на его заказ и подтверждаем открытие тактильно —
  // так переход из уведомления ощущается как продолжение, а не как рестарт.
  useEffect(() => {
    const orderId = orderFromStartParam();
    if (!orderId) return;
    haptic('light');
    navigate(`/confirmation/${orderId}`, { replace: true });
    // Намеренно один раз за сессию: start_param не меняется без перезапуска.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Кнопка «Назад» Telegram на всех страницах, кроме главной.
  useEffect(() => {
    const app = tg();
    if (!app) return;
    const goBack = () => navigate(-1);
    if (location.pathname === '/') {
      app.BackButton.hide();
    } else {
      app.BackButton.show();
      app.BackButton.onClick(goBack);
    }
    return () => app.BackButton.offClick(goBack);
  }, [location.pathname, navigate]);

  // Скроллим к началу при переходах.
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [location.pathname]);

  // Нижнее меню — на просмотровых экранах. Не в корзине (там свой нижний бар),
  // оформлении, карточке товара.
  const showTabBar = ['/', '/catalog', '/profile'].includes(location.pathname);

  return (
    <div className="mx-auto min-h-screen max-w-md font-sans text-ink">
      <div className="ambient" aria-hidden />
      <div className="grain" aria-hidden />
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/catalog" element={<Catalog />} />
        <Route path="/product/:id" element={<ProductPage />} />
        <Route path="/cart" element={<Cart />} />
        <Route path="/checkout" element={<Checkout />} />
        <Route path="/confirmation/:id" element={<Confirmation />} />
        <Route path="/profile" element={<Profile />} />
      </Routes>
      {showTabBar && <TabBar />}
      <AppLoader done={booted} />
    </div>
  );
}
