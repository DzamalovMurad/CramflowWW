import { useEffect } from 'react';
import { Route, Routes, useLocation, useNavigate } from 'react-router-dom';
import Home from './pages/Home';
import Catalog from './pages/Catalog';
import ProductPage from './pages/Product';
import Cart from './pages/Cart';
import Checkout from './pages/Checkout';
import Confirmation from './pages/Confirmation';
import Profile from './pages/Profile';
import TabBar from './components/TabBar';
import { tg } from './telegram';

export default function App() {
  const location = useLocation();
  const navigate = useNavigate();

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
    </div>
  );
}
