import { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Stepper from '../components/Stepper';
import SwipeToDelete from '../components/SwipeToDelete';
import { useCart } from '../cart';
import { tg } from '../telegram';
import { content } from '../content';
import { formatPrice } from '../types';
import { IconBag, IconClose } from '../components/icons';

/** Корзина: строки с hairline-разделителями, итого, оформление. */
export default function Cart() {
  const { items, total, setQty, remove } = useCart();
  const navigate = useNavigate();
  const { state } = useLocation() as { state?: { notice?: string } };

  // Сообщение приходит с экрана оформления, когда часть букетов раскупили,
  // пока клиент заполнял форму. Показываем один раз и убираем из истории,
  // чтобы оно не всплывало снова при возврате «назад».
  const [notice, setNotice] = useState(state?.notice ?? '');
  useEffect(() => {
    if (!state?.notice) return;
    navigate('.', { replace: true, state: {} });
    const t = setTimeout(() => setNotice(''), 6000);
    return () => clearTimeout(t);
  }, [state?.notice, navigate]);

  if (items.length === 0) {
    return (
      <div>
        <Header title={content.cart.title} showBack={!tg()} />
        {notice && <Notice text={notice} />}
        <div className="flex flex-col items-center px-10 py-24 text-center">
          <div className="animate-pop-in flex h-16 w-16 items-center justify-center rounded-full bg-tile text-muted">
            <IconBag size={26} />
          </div>
          <p className="mt-5 text-[17px] font-semibold lowercase">{content.cart.empty}</p>
          <p className="mt-1 text-sm lowercase text-muted">{content.cart.emptyHint}</p>
          <Link
            to="/catalog"
            className="mt-7 flex min-h-[48px] items-center rounded-button bg-ink px-7 text-sm font-semibold lowercase text-page"
          >
            {content.cart.toCatalog}
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div className="pb-36">
      <Header title={content.cart.title} showBack={!tg()} />
      {notice && <Notice text={notice} />}

      <div className="space-y-3 px-4 pt-3">
        {items.map((item, i) => (
          <SwipeToDelete key={item.variantId} onDelete={() => remove(item.variantId)}>
            <div
              className="cart-row animate-fade-up flex gap-3.5 p-3.5"
              style={{ animationDelay: `${Math.min(i * 50, 200)}ms` }}
            >
              <Link to={`/product/${item.productId}`} className="flex-shrink-0">
                <div className="h-[88px] w-[72px] overflow-hidden rounded-card bg-tile">
                  {item.image && (
                    <img
                      src={item.image}
                      alt={item.productName}
                      width={72}
                      height={88}
                      loading="lazy"
                      className="h-full w-full object-cover"
                    />
                  )}
                </div>
              </Link>
              <div className="flex min-w-0 flex-1 flex-col">
                <div className="flex items-start justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-[15px] font-medium">{item.productName}</p>
                    <p className="mt-0.5 text-xs lowercase text-muted">
                      {item.flowersCount} {content.product.flowersUnit} {content.cart.inBouquet}
                    </p>
                  </div>
                  <button
                    onClick={() => remove(item.variantId)}
                    aria-label="удалить"
                    className="-mr-2 -mt-2 flex h-11 w-11 flex-shrink-0 items-center justify-center text-muted active:opacity-50"
                  >
                    <IconClose size={16} />
                  </button>
                </div>
                <div className="mt-auto flex items-center justify-between pt-2">
                  <Stepper value={item.qty} onChange={(v) => setQty(item.variantId, v)} />
                  <span className="price text-[17px]">
                    {formatPrice(item.price * item.qty)}
                  </span>
                </div>
              </div>
            </div>
          </SwipeToDelete>
        ))}
      </div>

      <div className="cart-total pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 bg-page/95 px-4 pt-3.5 backdrop-blur">
        <div className="mb-3.5 flex items-end justify-between">
          <span className="label mb-1">{content.cart.total}</span>
          <span className="price price-total">{formatPrice(total)}</span>
        </div>
        <button
          onClick={() => navigate('/checkout')}
          className="cta btn-accent min-h-[58px] w-full rounded-button text-[15px] text-on-accent"
        >
          {content.cart.checkout}
        </button>
      </div>
    </div>
  );
}

function Notice({ text }: { text: string }) {
  return (
    <p
      role="status"
      className="animate-fade-in mx-4 mt-3 rounded-input border border-accent/40 bg-accent/10 px-4 py-3 text-sm lowercase leading-snug text-ink"
    >
      {text}
    </p>
  );
}
