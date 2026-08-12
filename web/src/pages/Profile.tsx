import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import Header from '../components/Header';
import { fetchMe, fetchMyOrders } from '../api';
import { content, orderStatusLabels } from '../content';
import { deliveryLine, deliveryTimeLabel, formatPrice, type Order } from '../types';

const c = content.profile;

interface Me {
  name?: string;
  phone?: string;
  promo_code?: string;
  discount_percent?: number;
}

/** Профиль: сохранённые имя/телефон и промокод из deep-link. */
export default function Profile() {
  const [me, setMe] = useState<Me | null>(null);
  const [orders, setOrders] = useState<Order[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    fetchMe()
      .then(setMe)
      .catch(() => {})
      .finally(() => setLoaded(true));
    fetchMyOrders()
      .then(setOrders)
      .catch(() => {});
  }, []);

  const hasData = me && (me.name || me.phone || me.promo_code);

  return (
    <div className="pb-28">
      <Header title={c.title} />

      <div className="p-4">
        {loaded && !hasData && (
          <div className="animate-fade-up rounded-card bg-surface p-6 text-center shadow-card">
            <p className="text-[15px] font-bold">{c.guest}</p>
            <p className="mt-1.5 text-sm text-muted">{c.guestHint}</p>
            <Link
              to="/catalog"
              className="btn-accent mt-5 inline-block rounded-button px-6 py-3 text-[14px] font-bold lowercase"
            >
              {c.toCatalog}
            </Link>
          </div>
        )}

        {hasData && (
          <div className="animate-fade-up space-y-3">
            {me!.name && (
              <Row label={c.name} value={me!.name} />
            )}
            {me!.phone && <Row label={c.phone} value={me!.phone} />}
            {me!.promo_code && (
              <div className="rounded-card bg-ink p-5 text-page shadow-card">
                <p className="text-[11px] font-bold uppercase tracking-wider opacity-70">{c.promo}</p>
                <div className="mt-1 flex items-baseline justify-between">
                  <span className="text-[22px] font-extrabold text-accent-ink">{me!.promo_code}</span>
                  <span className="text-sm opacity-80">
                    {c.discount} {me!.discount_percent}%
                  </span>
                </div>
              </div>
            )}
          </div>
        )}

        {orders.length > 0 && (
          <div className="mt-6">
            <p className="label mb-2.5">{c.history}</p>
            <div className="space-y-3">
              {orders.map((o) => (
                <OrderRow key={o.id} order={o} />
              ))}
            </div>
          </div>
        )}

        <div className="mt-6 rounded-card bg-surface p-5 shadow-card">
          <p className="label mb-2">{c.about}</p>
          <p className="text-sm leading-relaxed text-muted">{c.aboutText}</p>
        </div>
      </div>
    </div>
  );
}

/** Карточка заказа в истории: сумма — только букеты, доставка описана словами. */
function OrderRow({ order }: { order: Order }) {
  return (
    <div className="animate-fade-up rounded-card bg-surface p-5 shadow-card">
      <div className="flex items-baseline justify-between gap-3">
        <span className="text-[15px] font-bold">№ {order.id}</span>
        <span className="text-xs lowercase text-muted">{orderStatusLabels[order.status] ?? order.status}</span>
      </div>
      <div className="mt-2 space-y-1 text-sm">
        {order.items.map((item) => (
          <div key={item.id} className="flex items-baseline justify-between gap-3">
            <span className="lowercase">
              {item.product_name} · {item.variant.quantity} шт
              {item.quantity > 1 && <span className="text-muted"> ×{item.quantity}</span>}
            </span>
            <span className="whitespace-nowrap font-mono">{formatPrice(item.price * item.quantity)}</span>
          </div>
        ))}
      </div>
      <div className="mt-3 flex items-baseline justify-between border-t border-line pt-3">
        <span className="text-sm lowercase text-muted">{content.confirmation.total}</span>
        <span className="font-mono text-[15px] font-bold">{formatPrice(order.total_price)}</span>
      </div>
      {/* Та же формулировка, что в подтверждении и в боте. */}
      <p className="mt-2.5 text-[13px] leading-relaxed text-ink">{deliveryLine(order)}</p>
      <p className="mt-0.5 text-xs lowercase leading-relaxed text-muted">
        {order.delivery_type !== 'metro' && order.delivery_address && `${order.delivery_address} · `}
        {order.delivery_date}, {deliveryTimeLabel(order)}
      </p>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between rounded-card bg-surface px-5 py-4 shadow-card">
      <span className="label">{label}</span>
      <span className="text-[15px] font-bold">{value}</span>
    </div>
  );
}
