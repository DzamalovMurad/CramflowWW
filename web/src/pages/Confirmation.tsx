import { useEffect, useState } from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import Header from '../components/Header';
import OrderCard from '../components/OrderCard';
import { fetchOrder } from '../api';
import { content } from '../content';
import type { Order } from '../types';
import { IconCheck } from '../components/icons';

const c = content.confirmation;

/** Экран подтверждения: номер заказа, состав, что будет дальше. */
export default function Confirmation() {
  const { id } = useParams();
  const { state } = useLocation() as { state?: { order?: Order } };

  // Заказ приходит из перехода после оформления, но экран должен
  // работать и при перезагрузке, и при возврате по ссылке — тогда
  // подтягиваем его с сервера.
  const [order, setOrder] = useState<Order | null>(state?.order ?? null);
  const [error, setError] = useState('');

  useEffect(() => {
    if (order || !id) return;
    let cancelled = false;
    fetchOrder(id)
      .then((o) => !cancelled && setOrder(o))
      .catch((e) => !cancelled && setError((e as Error).message));
    return () => {
      cancelled = true;
    };
  }, [id, order]);

  return (
    <div className="pb-10">
      <Header />
      <div className="flex flex-col items-center px-6 pt-12 text-center">
        <div className="animate-pop-in flex h-16 w-16 items-center justify-center rounded-full bg-accent text-on-accent">
          <IconCheck size={26} />
        </div>
        <h1 className="title animate-fade-up mt-7">{c.title}</h1>
        <p
          className="animate-fade-up mt-2 text-sm lowercase text-muted"
          style={{ animationDelay: '80ms' }}
        >
          {content.order.number} <span className="font-semibold text-ink">#{id}</span> · {c.subtitle}
        </p>
      </div>

      <div className="animate-fade-up mx-4 mt-8" style={{ animationDelay: '160ms' }}>
        {order && <OrderCard order={order} />}
        {!order && !error && (
          <div className="space-y-3 rounded-card border border-line bg-surface p-5">
            <div className="h-4 w-1/3 animate-pulse rounded bg-tile" />
            <div className="h-3 w-full animate-pulse rounded bg-tile" />
            <div className="h-3 w-2/3 animate-pulse rounded bg-tile" />
          </div>
        )}
        {error && <p className="text-center text-sm lowercase text-muted">{error}</p>}
      </div>

      <p
        className="animate-fade-up mt-8 px-10 text-center text-sm lowercase leading-relaxed text-muted"
        style={{ animationDelay: '240ms' }}
      >
        {c.managerCall}
      </p>

      <div className="px-4 pt-8">
        <Link
          to="/catalog"
          className="flex min-h-[52px] w-full items-center justify-center rounded-button bg-ink text-[15px] font-semibold lowercase text-page transition-transform active:scale-[0.98]"
        >
          {c.backToCatalog}
        </Link>
      </div>
    </div>
  );
}
