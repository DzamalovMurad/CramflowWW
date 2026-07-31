import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Stepper from '../components/Stepper';
import SwipeToDelete from '../components/SwipeToDelete';
import ProductCardView from '../components/ProductCardView';
import { ShelfSkeleton } from '../components/Skeletons';
import { fetchHits, fetchProduct } from '../api';
import { useCart } from '../cart';
import { haptic, tg } from '../telegram';
import { content } from '../content';
import { formatPrice, type ProductCard as ProductCardType } from '../types';
import { IconBag, IconClose } from '../components/icons';

/**
 * EmptyCart — пустая корзина не тупик: сразу показываем пятёрку хитов
 * (порядок задаёт sort_order в админке) и уводим в них одним тапом.
 * Подборка грузится молча — если сети нет, остаётся обычная кнопка в каталог.
 */
function EmptyCart() {
  const [hits, setHits] = useState<ProductCardType[] | null>(null);
  const [failed, setFailed] = useState(false);
  const { add } = useCart();

  useEffect(() => {
    let alive = true;
    fetchHits(5)
      .then((list) => alive && setHits(list))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, []);

  const addCheapest = async (card: ProductCardType) => {
    const product = await fetchProduct(card.id);
    const variant = product.variants[0];
    if (!variant) return;
    add({
      variantId: variant.id,
      productId: product.id,
      productName: product.name,
      flowersCount: variant.quantity,
      price: variant.price,
      image: product.images[0]?.url ?? '',
    });
    haptic('success');
  };

  const scrollToHits = () => {
    haptic('light');
    document.getElementById('cart-hits')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  const hasHits = hits !== null && hits.length > 0;

  return (
    <div className="pb-24">
      <Header title={content.cart.title} showBack={!tg()} />
      <div className="flex flex-col items-center px-10 pb-10 pt-20 text-center">
        <div className="animate-pop-in flex h-16 w-16 items-center justify-center rounded-full bg-tile text-muted">
          <IconBag size={26} />
        </div>
        <p className="mt-5 text-[17px] font-bold lowercase">{content.cart.empty}</p>
        <p className="mt-1 text-sm lowercase text-muted">{content.cart.emptyHint}</p>

        {hasHits ? (
          <button
            type="button"
            onClick={scrollToHits}
            className="btn-accent mt-7 rounded-button px-7 py-3.5 text-sm font-bold lowercase text-on-accent"
          >
            {content.cart.seeHits}
          </button>
        ) : (
          <Link
            to="/catalog"
            className="mt-7 rounded-button bg-ink px-7 py-3.5 text-sm font-bold lowercase text-page"
          >
            {content.cart.toCatalog}
          </Link>
        )}
      </div>

      {/* Подборка хитов: скелетон, пока грузится; при ошибке просто ничего. */}
      {hits === null && !failed && <ShelfSkeleton />}
      {hasHits && (
        <section id="cart-hits" className="scroll-mt-4">
          <h2 className="display px-4 pb-3 text-[22px]">{content.cart.hitsTitle}</h2>
          <div className="grid grid-cols-2 gap-x-3 gap-y-6 px-4">
            {hits.map((p, i) => (
              <ProductCardView key={p.id} product={p} index={i} onAdd={addCheapest} />
            ))}
          </div>
        </section>
      )}
    </div>
  );
}

/** Корзина: строки с hairline-разделителями, итого, оформление. */
export default function Cart() {
  const { items, total, setQty, remove } = useCart();
  const navigate = useNavigate();

  if (items.length === 0) {
    return <EmptyCart />;
  }

  return (
    <div className="pb-36">
      <Header title={content.cart.title} showBack={!tg()} />

      <div className="divide-y divide-line">
        {items.map((item, i) => (
          <SwipeToDelete key={item.variantId} onDelete={() => remove(item.variantId)}>
            <div
              className="animate-fade-up flex gap-3.5 px-4 py-4"
              style={{ animationDelay: `${i * 50}ms` }}
            >
              <Link to={`/product/${item.productId}`} className="flex-shrink-0">
                <div className="h-[88px] w-[72px] overflow-hidden rounded-card bg-tile">
                  {item.image && (
                    <img
                      src={item.image}
                      alt={item.productName}
                      loading="lazy"
                      decoding="async"
                      width={72}
                      height={88}
                      className="h-full w-full object-cover"
                    />
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
                  <span className="font-mono text-[15px] font-bold">{formatPrice(item.price * item.qty)}</span>
                </div>
              </div>
            </div>
          </SwipeToDelete>
        ))}
      </div>

      <div className="pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 border-t border-line bg-page/95 px-4 pt-3 backdrop-blur">
        <div className="mb-3 flex items-baseline justify-between">
          <span className="text-sm lowercase text-muted">{content.cart.total}</span>
          <span className="font-mono text-xl font-bold">{formatPrice(total)}</span>
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
