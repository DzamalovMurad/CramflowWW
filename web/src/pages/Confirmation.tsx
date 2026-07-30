import { Link, useLocation, useParams } from 'react-router-dom';
import Header from '../components/Header';
import { content } from '../content';
import { formatPrice, type Order } from '../types';
import { IconCheck } from '../components/icons';

const c = content.confirmation;

/** Экран подтверждения: номер заказа, состав, «ожидайте звонка менеджера». */
export default function Confirmation() {
  const { id } = useParams();
  const { state } = useLocation();
  const order: Order | undefined = state?.order;

  return (
    <div className="pb-10">
      <Header />
      <div className="flex flex-col items-center px-6 pt-14 text-center">
        <div className="animate-pop-in flex h-16 w-16 items-center justify-center rounded-full bg-accent text-on-accent">
          <IconCheck size={26} />
        </div>
        <h1 className="display animate-fade-up mt-6 text-[26px]">{c.title}</h1>
        <p className="animate-fade-up mt-2 text-sm lowercase text-muted" style={{ animationDelay: '80ms' }}>
          {c.orderLabel} <span className="font-bold text-ink">#{id}</span> {c.orderAccepted}
        </p>
      </div>

      {order && (
        <div
          className="animate-fade-up mx-4 mt-9 rounded-card bg-tile p-5"
          style={{ animationDelay: '160ms' }}
        >
          <p className="label mb-3">{c.composition}</p>
          <div className="space-y-2.5">
            {order.items.map((item) => (
              <div key={item.id} className="flex items-baseline justify-between gap-3 text-sm">
                <span className="lowercase">
                  {item.product_name} · {item.variant.quantity} шт
                  {item.quantity > 1 && <span className="text-muted"> ×{item.quantity}</span>}
                </span>
                <span className="whitespace-nowrap font-mono font-medium">
                  {formatPrice(item.price * item.quantity)}
                </span>
              </div>
            ))}
          </div>
          {order.promo_code && (
            <p className="mt-3.5 flex items-center gap-2 text-sm lowercase text-accent-2">
              <span className="h-1.5 w-1.5 rounded-full bg-accent-2" />
              {c.promoApplied}: <span className="normal-case">{order.promo_code.code}</span>
              {order.discount_amount ? ` (−${formatPrice(order.discount_amount)})` : ''}
            </p>
          )}
          <div className="mt-4 flex justify-between border-t border-line pt-3.5 font-bold lowercase">
            <span>{c.total}</span>
            <span className="font-mono">{formatPrice(order.total_price)}</span>
          </div>
          <p className="mt-3 text-xs lowercase leading-relaxed text-muted">
            {c.delivery}: {order.delivery_address} · {order.delivery_date}, {order.delivery_time}
          </p>
        </div>
      )}

      <p
        className="animate-fade-up mt-9 px-12 text-center text-sm lowercase leading-relaxed text-muted"
        style={{ animationDelay: '240ms' }}
      >
        {c.managerCall}
      </p>

      <div className="px-4 pt-9">
        <Link
          to="/catalog"
          className="block w-full rounded-button bg-ink py-4 text-center text-[15px] font-bold lowercase text-page transition-transform active:scale-[0.98]"
        >
          {c.backToCatalog}
        </Link>
      </div>
    </div>
  );
}
