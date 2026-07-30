import { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import Stepper from '../components/Stepper';
import SwipeToDelete from '../components/SwipeToDelete';
import { fetchAddons, fetchProduct } from '../api';
import { useCart } from '../cart';
import { haptic, pendingPromo, tg } from '../telegram';
import { content } from '../content';
import { formatPrice, type AddonCard, type ProductVariant } from '../types';
import { IconBag, IconClose, IconPlus } from '../components/icons';

/** Корзина: строки с hairline-разделителями, размеры, допродажи, итого, оформление. */
export default function Cart() {
  const { items, total, add, setQty, remove, changeVariant } = useCart();
  const navigate = useNavigate();
  const { state } = useLocation();
  // «Повторить заказ» мог добавить не всё — показываем, чего не хватает.
  const missing: string[] = state?.missing ?? [];
  const promoCode = pendingPromo();

  const [addons, setAddons] = useState<AddonCard[]>([]);
  useEffect(() => {
    fetchAddons()
      .then(setAddons)
      .catch(() => {});
  }, []);

  // Варианты товаров из корзины — для переключателя размера прямо в строке.
  const [variantsByProduct, setVariantsByProduct] = useState<Record<number, ProductVariant[]>>({});
  useEffect(() => {
    const ids = [...new Set(items.map((i) => i.productId))];
    const missingIds = ids.filter((id) => !(id in variantsByProduct));
    if (missingIds.length === 0) return;
    Promise.all(
      missingIds.map((id) =>
        fetchProduct(id)
          .then((p) => [id, p.variants] as const)
          .catch(() => [id, [] as ProductVariant[]] as const),
      ),
    ).then((pairs) => setVariantsByProduct((prev) => ({ ...prev, ...Object.fromEntries(pairs) })));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [items]);

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

      {missing.length > 0 && (
        <div className="animate-fade-in mx-4 mb-1 mt-3 rounded-card border border-line bg-tile px-4 py-3 text-xs lowercase text-muted">
          {content.cart.repeatMissing}: {missing.join(', ')}
        </div>
      )}

      {promoCode && (
        <div className="animate-fade-in mx-4 mb-1 mt-3 flex items-center gap-2 rounded-card border border-accent/40 bg-surface px-4 py-3 text-xs lowercase text-ink">
          <span className="h-1.5 w-1.5 flex-shrink-0 rounded-full bg-accent" />
          {content.cart.promoBanner.replace('{code}', promoCode)}
        </div>
      )}

      <div className="divide-y divide-line">
        {items.map((item, i) => {
          const variants = variantsByProduct[item.productId] ?? [];
          return (
            <SwipeToDelete key={item.variantId} onDelete={() => remove(item.variantId)}>
              <div
                className="animate-fade-up flex gap-3.5 px-4 py-4"
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
                  {variants.length > 1 && (
                    <div className="mt-2 flex flex-wrap gap-1.5">
                      {variants.map((v) => {
                        const active = v.id === item.variantId;
                        return (
                          <button
                            key={v.id}
                            onClick={() => {
                              if (active) return;
                              haptic('light');
                              changeVariant(item.variantId, {
                                variantId: v.id,
                                flowersCount: v.quantity,
                                price: v.price,
                              });
                            }}
                            className={`rounded-button border px-2.5 py-1 font-mono text-[11px] font-bold transition-colors ${
                              active ? 'border-ink bg-ink text-page' : 'border-line bg-page text-muted'
                            }`}
                          >
                            {v.quantity} {content.product.flowersUnit}
                          </button>
                        );
                      })}
                    </div>
                  )}
                  <div className="mt-auto flex items-center justify-between pt-2">
                    <Stepper value={item.qty} onChange={(v) => setQty(item.variantId, v)} />
                    <span className="font-mono text-[15px] font-bold">{formatPrice(item.price * item.qty)}</span>
                  </div>
                </div>
              </div>
            </SwipeToDelete>
          );
        })}
      </div>

      {addons.length > 0 && (
        <div className="mt-2 border-t border-line pt-4">
          <p className="label px-4">{content.cart.addons}</p>
          <div className="mt-2.5 flex gap-3 overflow-x-auto px-4 pb-2 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {addons.map((a) => (
              <div key={a.id} className="w-[120px] flex-shrink-0">
                <div className="relative h-[120px] w-[120px] overflow-hidden rounded-card bg-tile">
                  {a.image && <img src={a.image} alt={a.name} className="h-full w-full object-cover" />}
                  <button
                    onClick={() => {
                      haptic('success');
                      add({
                        variantId: a.variant_id,
                        productId: a.id,
                        productName: a.name,
                        flowersCount: a.flowers_count,
                        price: a.price,
                        image: a.image,
                      });
                    }}
                    aria-label={`добавить ${a.name}`}
                    className="btn-accent absolute bottom-2 right-2 flex h-8 w-8 items-center justify-center rounded-full text-on-accent active:scale-90"
                  >
                    <IconPlus size={15} />
                  </button>
                </div>
                <p className="mt-1.5 truncate text-xs font-medium lowercase">{a.name}</p>
                <p className="font-mono text-xs font-bold">{formatPrice(a.price)}</p>
              </div>
            ))}
          </div>
        </div>
      )}

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
