import { Link, useLocation, useParams } from 'react-router-dom';
import Header from '../components/Header';
import { formatPrice, type Order } from '../types';

/** Экран подтверждения: номер заказа, состав, «ожидайте звонка менеджера». */
export default function Confirmation() {
  const { id } = useParams();
  const { state } = useLocation();
  const order: Order | undefined = state?.order;

  return (
    <div className="pb-10">
      <Header />
      <div className="flex flex-col items-center px-6 pt-12 text-center">
        <div className="animate-pop-in flex h-20 w-20 items-center justify-center rounded-full bg-accent/20 text-4xl">
          🌸
        </div>
        <h1 className="animate-fade-up mt-5 text-2xl font-bold">Спасибо за ваш заказ!</h1>
        <p className="animate-fade-up mt-1 text-muted" style={{ animationDelay: '80ms' }}>
          Заказ <span className="font-semibold text-ink">#{id}</span> принят
        </p>
      </div>

      {order && (
        <div
          className="animate-fade-up mx-4 mt-8 rounded-card border border-line bg-surface p-4 shadow-card"
          style={{ animationDelay: '160ms' }}
        >
          <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-muted">
            Состав заказа
          </h2>
          <div className="space-y-2">
            {order.items.map((item) => (
              <div key={item.id} className="flex items-baseline justify-between gap-3 text-sm">
                <span>
                  {item.product_name} · {item.variant.quantity} шт
                  {item.quantity > 1 && <span className="text-muted"> ×{item.quantity}</span>}
                </span>
                <span className="whitespace-nowrap font-medium">
                  {formatPrice(item.price * item.quantity)}
                </span>
              </div>
            ))}
          </div>
          {order.promo_code && (
            <p className="mt-3 text-sm text-accent-2">
              Промокод {order.promo_code.code} (−{order.promo_code.discount_percent}%) применён
            </p>
          )}
          <div className="mt-3 flex justify-between border-t border-line pt-3 font-bold">
            <span>Итого</span>
            <span>{formatPrice(order.total_price)}</span>
          </div>
          <p className="mt-3 text-xs text-muted">
            Доставка: {order.delivery_address} · {order.delivery_date}, {order.delivery_time}
          </p>
        </div>
      )}

      <p className="animate-fade-up mt-8 px-10 text-center text-sm text-muted" style={{ animationDelay: '240ms' }}>
        📞 Ожидайте звонка менеджера для подтверждения заказа
      </p>

      <div className="px-4 pt-8">
        <Link
          to="/catalog"
          className="block w-full rounded-card border border-line py-3.5 text-center text-sm font-semibold active:bg-line"
        >
          Вернуться в каталог
        </Link>
      </div>
    </div>
  );
}
