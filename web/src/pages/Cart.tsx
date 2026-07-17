import { Link, useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Stepper from '../components/Stepper';
import { useCart } from '../cart';
import { tg } from '../telegram';
import { content } from '../content';
import { formatPrice } from '../types';
import { IconBag, IconClose } from '../components/icons';

/** Корзина: строки с hairline-разделителями, итого, оформление. */
export default function Cart() {
  const { items, total, setQty, remove } = useCart();
  const navigate = useNavigate();

  if (items.length === 0) {
    return (
      <div>
        <Header title={content.cart.title} showBack={!tg()} />
        <div className="flex flex-col items-center px-10 py-24 text-center">
          <div className="animate-pop-in flex h-16 w-16 items-center justify-center rounded-full bg-tile text-muted">
            <IconBag size={26} />
          </div>
          <p className="mt-5 text-[17px] font-bold lowercase">{content.cart.empty}</p>
          <p className="mt-1 text-sm lowercase text-muted">{content.cart.emptyHint}</p>
          <Link
            to="/catalog"
            className="mt-7 rounded-button bg-ink px-7 py-3.5 text-sm font-bold lowercase text-page"
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

      <div className="divide-y divide-line px-4">
        {items.map((item, i) => (
          <div
            key={item.variantId}
            className="animate-fade-up flex gap-3.5 py-4"
            style={{ animationDelay: `${i * 50}ms` }}
          >
            <Link to={`/product/${item.productId}`} className="flex-shrink-0">
              <div className="h-[88px] w-[72px] overflow-hidden rounded-card bg-tile">
                {item.image && (
                  <img src={item.image} alt={item.productName} className="h-full w-full object-cover" />
                )}
              </div>
            </Link>
            <div className="flex min-w-0 flex-1 flex-col">
              <div className="flex items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium lowercase">{item.productName}</p>
                  <p className="mt-0.5 text-xs lowercase text-muted">
                    {item.flowersCount} {content.product.flowersUnit} {content.cart.inBouquet}
                  </p>
                </div>
                <button
                  onClick={() => remove(item.variantId)}
                  aria-label="удалить"
                  className="-mr-1 flex h-7 w-7 items-center justify-center text-muted active:opacity-50"
                >
                  <IconClose size={15} />
                </button>
              </div>
              <div className="mt-auto flex items-center justify-between pt-2">
                <Stepper value={item.qty} onChange={(v) => setQty(item.variantId, v)} />
                <span className="text-[15px] font-bold">{formatPrice(item.price * item.qty)}</span>
              </div>
            </div>
          </div>
        ))}
      </div>

      <div className="pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 border-t border-line bg-page/95 px-4 pt-3 backdrop-blur">
        <div className="mb-3 flex items-baseline justify-between">
          <span className="text-sm lowercase text-muted">{content.cart.total}</span>
          <span className="text-xl font-bold">{formatPrice(total)}</span>
        </div>
        <button
          onClick={() => navigate('/checkout')}
          className="btn-accent w-full rounded-button py-4 text-[15px] font-bold lowercase text-on-accent"
        >
          {content.cart.checkout}
        </button>
      </div>
    </div>
  );
}
