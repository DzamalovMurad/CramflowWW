import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import { fetchMe, fetchMyOrders, fetchRepeatOrder } from '../api';
import { useCart } from '../cart';
import { haptic } from '../telegram';
import { content } from '../content';
import { formatPrice, STATUS_LABELS, type Order } from '../types';

const c = content.profile;

interface Me {
  name?: string;
  phone?: string;
  promo_code?: string;
  promo_label?: string;
}

/** Профиль: сохранённые имя/телефон, промокод из deep-link и история заказов. */
export default function Profile() {
  const [me, setMe] = useState<Me | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [orders, setOrders] = useState<Order[]>([]);
  const { fill } = useCart();
  const navigate = useNavigate();

  // id заказа, который сейчас «повторяется», и ошибки повторов по заказам.
  const [repeating, setRepeating] = useState<number | null>(null);
  const [repeatErrors, setRepeatErrors] = useState<Record<number, string>>({});

  useEffect(() => {
    fetchMe()
      .then(setMe)
      .catch(() => {})
      .finally(() => setLoaded(true));
    fetchMyOrders()
      .then(setOrders)
      .catch(() => {});
  }, []);

  // «Повторить заказ»: сервер сверяет позиции с актуальным каталогом,
  // корзина заполняется доступными, о недоступных предупреждаем в корзине.
  const repeat = async (orderId: number) => {
    setRepeating(orderId);
    setRepeatErrors((prev) => ({ ...prev, [orderId]: '' }));
    try {
      const { items } = await fetchRepeatOrder(orderId);
      const available = items.filter((i) => i.available);
      const missing = items.filter((i) => !i.available).map((i) => i.product_name);
      if (available.length === 0) {
        setRepeatErrors((prev) => ({ ...prev, [orderId]: c.repeatEmpty }));
        return;
      }
      fill(
        available.map((i) => ({
          variantId: i.variant_id!,
          productId: i.product_id!,
          productName: i.product_name,
          flowersCount: i.flowers_count!,
          price: i.price!,
          image: i.image ?? '',
          qty: i.qty,
        })),
      );
      haptic('success');
      navigate('/cart', missing.length > 0 ? { state: { missing } } : undefined);
    } catch (e) {
      setRepeatErrors((prev) => ({ ...prev, [orderId]: (e as Error).message }));
    } finally {
      setRepeating(null);
    }
  };

  const hasData = me && (me.name || me.phone || me.promo_code);

  return (
    <div className="pb-28">
      <Header title={c.title} />

      <div className="p-4">
        {loaded && !hasData && orders.length === 0 && (
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
                    {c.discount} {me!.promo_label}
                  </span>
                </div>
              </div>
            )}
          </div>
        )}

        {orders.length > 0 && (
          <div className="animate-fade-up mt-6">
            <p className="label mb-2">{c.orders}</p>
            <div className="space-y-3">
              {orders.map((o) => (
                <div key={o.id} className="rounded-card bg-surface p-4 shadow-card">
                  <div className="flex items-baseline justify-between">
                    <span className="text-sm font-bold lowercase">
                      {c.orderLabel} #{o.id}
                    </span>
                    <span className="text-xs lowercase text-muted">{STATUS_LABELS[o.status] ?? o.status}</span>
                  </div>
                  <p className="mt-1 truncate text-xs lowercase text-muted">
                    {o.items.map((it) => it.product_name).join(', ')}
                  </p>
                  <div className="mt-2.5 flex items-center justify-between">
                    <span className="font-mono text-[15px] font-bold">{formatPrice(o.total_price)}</span>
                    <button
                      onClick={() => repeat(o.id)}
                      disabled={repeating !== null}
                      className="rounded-button border border-line bg-page px-4 py-2 text-xs font-bold lowercase text-ink transition-transform active:scale-95 disabled:opacity-50"
                    >
                      {repeating === o.id ? c.repeating : c.repeat}
                    </button>
                  </div>
                  {repeatErrors[o.id] && (
                    <p className="mt-2 text-xs lowercase text-red-500">{repeatErrors[o.id]}</p>
                  )}
                </div>
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

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between rounded-card bg-surface px-5 py-4 shadow-card">
      <span className="label">{label}</span>
      <span className="text-[15px] font-bold">{value}</span>
    </div>
  );
}
