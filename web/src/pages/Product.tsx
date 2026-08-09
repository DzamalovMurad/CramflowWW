import { useEffect, useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router-dom';
import Header from '../components/Header';
import Gallery from '../components/Gallery';
import Stepper from '../components/Stepper';
import { fetchProduct } from '../api';
import { useCart } from '../cart';
import { haptic, tg } from '../telegram';
import { content, categoryLabels } from '../content';
import { discountPercent, formatPrice, inStock, isLowStock, type Product } from '../types';

/** Карточка товара: галерея, варианты-чипы, количество, нижняя кнопка с суммой. */
export default function ProductPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { add } = useCart();

  const [product, setProduct] = useState<Product | null>(null);
  const [error, setError] = useState('');
  const [variantId, setVariantId] = useState<number | null>(null);
  const [qty, setQty] = useState(1);

  useEffect(() => {
    let cancelled = false;
    setProduct(null);
    setError('');
    fetchProduct(id!)
      .then((p) => {
        if (cancelled) return;
        setProduct(p);
        setVariantId(p.variants[0]?.id ?? null);
      })
      .catch((e) => !cancelled && setError((e as Error).message));
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (error) {
    return (
      <div>
        <Header showBack={!tg()} />
        <div className="flex flex-col items-center px-10 py-24 text-center">
          <p className="text-[17px] font-semibold lowercase">{error}</p>
          <Link
            to="/catalog"
            className="mt-6 flex min-h-[48px] items-center rounded-button bg-ink px-7 text-sm font-semibold lowercase text-page"
          >
            {content.product.toCatalog}
          </Link>
        </div>
      </div>
    );
  }

  if (!product) {
    return (
      <div>
        <Header showBack={!tg()} />
        <div className="aspect-square animate-pulse bg-tile" />
        <div className="space-y-3 p-5">
          <div className="h-7 w-2/3 animate-pulse rounded bg-tile" />
          <div className="h-4 w-full animate-pulse rounded bg-tile" />
          <div className="h-4 w-1/2 animate-pulse rounded bg-tile" />
        </div>
      </div>
    );
  }

  const variant = product.variants.find((v) => v.id === variantId) ?? product.variants[0];
  const total = (variant?.price ?? 0) * qty;
  const totalOld = variant?.old_price ? variant.old_price * qty : 0;
  const off = discountPercent(variant?.price ?? 0, variant?.old_price);
  const available = inStock(product) && Boolean(variant);
  const lowStock = isLowStock(product);
  // Больше остатка положить в корзину нельзя — иначе заказ отклонит сервер.
  const maxQty = typeof product.stock === 'number' && product.stock > 0 ? product.stock : 99;

  const addToCart = () => {
    if (!variant || !available) return;
    add(
      {
        variantId: variant.id,
        productId: product.id,
        productName: product.name,
        flowersCount: variant.quantity,
        price: variant.price,
        image: product.images[0]?.url ?? '',
      },
      Math.min(qty, maxQty),
    );
    haptic('success');
    navigate('/cart');
  };

  return (
    <div className="pb-32">
      <Header showBack={!tg()} />
      <Gallery images={product.images} alt={product.name} />

      <div className="p-5">
        <div className="flex flex-wrap items-center gap-1.5">
          <p className="label !text-[11px]">{categoryLabels[product.category] ?? product.category}</p>
          {product.is_hit && <span className="badge badge-hit">хит</span>}
          {off > 0 && <span className="badge badge-sale">−{off}%</span>}
          {lowStock && (
            <span className="badge badge-stock">
              {content.catalog.lastLeft} {product.stock}
            </span>
          )}
        </div>
        <h1 className="title mt-2">{product.name}</h1>

        {!available && (
          <div className="mt-4 rounded-input border border-line bg-tile p-4">
            <p className="text-[15px] font-semibold">{content.product.soldOut}</p>
            <p className="mt-1 text-sm lowercase text-muted">{content.product.soldOutHint}</p>
            <Link
              to="/catalog"
              className="mt-3 inline-flex min-h-[44px] items-center text-sm font-semibold text-accent-2"
            >
              {content.product.toCatalog} →
            </Link>
          </div>
        )}

        {product.description && (
          <p className="mt-3 text-sm leading-relaxed text-muted">{product.description}</p>
        )}
        <p className="mt-4 border-l-2 border-accent/30 pl-3 text-xs leading-relaxed text-muted">
          {content.product.availabilityNote}
        </p>

        {product.variants.length > 0 && (
          <>
            <p className="label mb-2.5 mt-7">{content.product.sizeLabel}</p>
            <div className="flex flex-wrap gap-2">
              {product.variants.map((v) => {
                const active = v.id === variant?.id;
                return (
                  <button
                    key={v.id}
                    onClick={() => {
                      haptic('light');
                      setVariantId(v.id);
                    }}
                    className={`min-h-[48px] rounded-button border px-4 text-sm font-medium transition-colors ${
                      active ? 'border-ink bg-ink text-page' : 'border-line bg-page text-ink'
                    }`}
                  >
                    {v.quantity} {content.product.flowersUnit} · {formatPrice(v.price)}
                  </button>
                );
              })}
            </div>
          </>
        )}

        {available && (
          <div className="mt-7 flex items-center justify-between">
            <p className="label">{content.product.qtyLabel}</p>
            <Stepper value={qty} max={maxQty} onChange={setQty} />
          </div>
        )}
      </div>

      <div className="pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 border-t border-line bg-page/95 px-4 pt-3 backdrop-blur">
        <button
          onClick={addToCart}
          disabled={!available}
          className="btn-accent flex min-h-[52px] w-full items-center justify-between rounded-button px-5 text-[15px] font-semibold text-on-accent disabled:opacity-50"
        >
          <span>{available ? content.product.addToCart : content.catalog.soldOut}</span>
          {available && (
            <span className="price flex items-baseline gap-2">
              {totalOld > 0 && (
                <span className="nums text-[13px] font-normal opacity-60 line-through">
                  {formatPrice(totalOld)}
                </span>
              )}
              {formatPrice(total)}
            </span>
          )}
        </button>
      </div>
    </div>
  );
}
