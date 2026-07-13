import { Link, useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Stepper from '../components/Stepper';
import { useCart } from '../cart';
import { tg } from '../telegram';
import { formatPrice } from '../types';

/** Корзина: позиции, итого, переход к оформлению. */
export default function Cart() {
  const { items, total, setQty, remove } = useCart();
  const navigate = useNavigate();

  if (items.length === 0) {
    return (
      <div>
        <Header showBack={!tg()} />
        <div className="flex flex-col items-center px-10 py-20 text-center">
          <p className="animate-pop-in text-5xl">🛒</p>
          <p className="mt-4 font-medium">Корзина пуста</p>
          <p className="mt-1 text-sm text-muted">Самое время выбрать букет</p>
          <Link
            to="/catalog"
            className="mt-6 rounded-card bg-accent px-6 py-3 text-sm font-semibold text-[#111111]"
          >
            В каталог
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="pb-32">
      <Header title="Корзина" showBack={!tg()} />

      <div className="space-y-3 p-4">
        {items.map((item, i) => (
          <div
            key={item.variantId}
            className="animate-fade-up flex gap-3 rounded-card border border-line bg-surface p-3 shadow-card"
            style={{ animationDelay: `${i * 50}ms` }}
          >
            <Link to={`/product/${item.productId}`} className="flex-shrink-0">
              <div className="h-20 w-20 overflow-hidden rounded-card bg-line">
                {item.image && (
                  <img src={item.image} alt={item.productName} className="h-full w-full object-cover" />
                )}
              </div>
            </Link>
            <div className="flex min-w-0 flex-1 flex-col">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{item.productName}</p>
                  <p className="text-xs text-muted">{item.flowersCount} шт в букете</p>
                </div>
                <button
                  onClick={() => remove(item.variantId)}
                  aria-label="Удалить"
                  className="-mr-1 -mt-1 flex h-7 w-7 items-center justify-center rounded-full text-muted active:bg-line"
                >
                  ×
                </button>
              </div>
              <div className="mt-auto flex items-center justify-between pt-2">
                <Stepper value={item.qty} onChange={(v) => setQty(item.variantId, v)} />
                <span className="text-base font-semibold">{formatPrice(item.price * item.qty)}</span>
              </div>
            </div>
          </div>
        ))}
      </div>

      <div className="pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 border-t border-line bg-page/95 px-4 pt-3 backdrop-blur">
        <div className="mb-3 flex items-center justify-between">
          <span className="text-sm text-muted">Итого</span>
          <span className="text-xl font-bold">{formatPrice(total)}</span>
        </div>
        <button
          onClick={() => navigate('/checkout')}
          className="w-full rounded-card bg-accent py-4 text-base font-semibold text-[#111111] shadow-card transition-transform active:scale-[0.98]"
        >
          Оформить заказ
        </button>
      </div>
    </div>
  );
}
