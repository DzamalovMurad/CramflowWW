import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import OrderCard from '../components/OrderCard';
import { fetchMe, fetchMyOrders, fetchProduct } from '../api';
import { useCart } from '../cart';
import { haptic } from '../telegram';
import { content } from '../content';
import { promoLabel, type CartItem, type Order } from '../types';

const c = content.profile;

interface Me {
  name?: string;
  phone?: string;
  promo_code?: string;
  discount_percent?: number;
  discount_type?: 'percent' | 'fixed';
  discount_value?: number;
  min_order_amount?: number;
}

/** Профиль: контакты, промокод, история заказов и повтор заказа в один тап. */
export default function Profile() {
  const [me, setMe] = useState<Me | null>(null);
  const [orders, setOrders] = useState<Order[] | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [repeatingID, setRepeatingID] = useState<number | null>(null);

  const { addMany } = useCart();
  const navigate = useNavigate();

  useEffect(() => {
    let cancelled = false;
    Promise.allSettled([fetchMe(), fetchMyOrders()]).then(([meRes, ordersRes]) => {
      if (cancelled) return;
      if (meRes.status === 'fulfilled') setMe(meRes.value);
      setOrders(ordersRes.status === 'fulfilled' ? ordersRes.value : []);
      setLoaded(true);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  /**
   * Повтор заказа — самый частый сценарий у цветочного: те же цветы тому же
   * человеку на следующий праздник.
   *
   * Корзина пересобирается по актуальному каталогу, а не по сохранённым ценам:
   * цены могли измениться, а варианты — быть заменены при правке (тогда ищем
   * букет того же размера). Недоступное молча пропускаем.
   */
  const repeat = async (order: Order) => {
    if (repeatingID !== null) return;
    setRepeatingID(order.id);
    haptic('medium');
    try {
      const found: Omit<CartItem, 'qty'>[] = [];
      const quantities: number[] = [];

      for (const item of order.items) {
        const product = await fetchProduct(item.product_id).catch(() => null);
        if (!product) continue;
        const variant =
          product.variants.find((v) => v.id === item.variant_id) ??
          product.variants.find((v) => v.quantity === item.flowers_count);
        if (!variant) continue;
        found.push({
          variantId: variant.id,
          productId: product.id,
          productName: product.name,
          flowersCount: variant.quantity,
          price: variant.price,
          image: product.images[0]?.url ?? '',
        });
        quantities.push(item.quantity);
      }

      if (found.length === 0) return;
      addMany(found, quantities);
      haptic('success');
      navigate('/cart');
    } finally {
      setRepeatingID(null);
    }
  };

  const hasProfile = Boolean(me && (me.name || me.phone || me.promo_code));
  const isNewcomer = loaded && !hasProfile && (orders?.length ?? 0) === 0;

  return (
    <div className="pb-28">
      <Header title={c.title} />

      <div className="space-y-6 p-4">
        {isNewcomer && (
          <div className="animate-fade-up rounded-card bg-surface p-6 text-center shadow-card">
            <p className="text-[15px] font-bold lowercase">{c.guest}</p>
            <p className="mt-1.5 text-sm lowercase text-muted">{c.guestHint}</p>
            <Link
              to="/catalog"
              className="btn-accent mt-5 inline-flex min-h-[48px] items-center rounded-button px-6 text-[14px] font-bold lowercase text-on-accent"
            >
              {c.toCatalog}
            </Link>
          </div>
        )}

        {!loaded && (
          <div className="space-y-3">
            {[0, 1].map((i) => (
              <div key={i} className="h-[68px] animate-pulse rounded-card bg-tile" />
            ))}
          </div>
        )}

        {hasProfile && (
          <div className="animate-fade-up space-y-3">
            {me?.name && <Row label={c.name} value={me.name} />}
            {me?.phone && <Row label={c.phone} value={me.phone} />}
            {me?.promo_code && (
              <div className="rounded-card bg-ink p-5 text-page shadow-card">
                <p className="text-[11px] font-bold uppercase tracking-wider opacity-70">
                  {c.promo}
                </p>
                <div className="mt-1 flex items-baseline justify-between">
                  <span className="text-[22px] font-extrabold text-accent-ink">{me.promo_code}</span>
                  <span className="text-sm opacity-80">
                    {c.discount}{' '}
                    {promoLabel({
                      code: me.promo_code,
                      discount_percent: me.discount_percent ?? 0,
                      discount_type: me.discount_type,
                      discount_value: me.discount_value,
                    })}
                  </span>
                </div>
              </div>
            )}
          </div>
        )}

        {orders && orders.length > 0 && (
          <section className="space-y-3">
            <h2 className="label">{c.orders}</h2>
            {orders.map((order) => (
              <div key={order.id} className="space-y-2">
                <OrderCard order={order} compact />
                <button
                  type="button"
                  onClick={() => repeat(order)}
                  disabled={repeatingID !== null}
                  className="min-h-[44px] w-full rounded-input border border-line bg-surface text-[14px] font-bold lowercase text-ink transition-transform active:scale-[0.98] disabled:opacity-50"
                >
                  {repeatingID === order.id ? c.repeatDone : c.repeat}
                </button>
              </div>
            ))}
          </section>
        )}

        <div className="rounded-card bg-surface p-5 shadow-card">
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
