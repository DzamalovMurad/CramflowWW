import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import Header from '../components/Header';
import Gallery from '../components/Gallery';
import Stepper from '../components/Stepper';
import { fetchProduct } from '../api';
import { useCart } from '../cart';
import { haptic, tg } from '../telegram';
import { formatPrice, type Product } from '../types';

/** Карточка товара: галерея, варианты, количество, нижняя кнопка с суммой. */
export default function ProductPage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { add } = useCart();

  const [product, setProduct] = useState<Product | null>(null);
  const [error, setError] = useState('');
  const [variantId, setVariantId] = useState<number | null>(null);
  const [qty, setQty] = useState(1);

  useEffect(() => {
    fetchProduct(id!)
      .then((p) => {
        setProduct(p);
        setVariantId(p.variants[0]?.id ?? null);
      })
      .catch((e) => setError(e.message));
  }, [id]);

  if (error) {
    return (
      <div>
        <Header showBack={!tg()} />
        <p className="p-10 text-center text-sm text-muted">{error}</p>
      </div>
    );
  }

  if (!product) {
    return (
      <div>
        <Header showBack={!tg()} />
        <div className="aspect-square animate-pulse bg-line" />
        <div className="space-y-3 p-4">
          <div className="h-6 w-2/3 animate-pulse rounded bg-line" />
          <div className="h-4 w-full animate-pulse rounded bg-line" />
        </div>
      </div>
    );
  }

  const variant = product.variants.find((v) => v.id === variantId) ?? product.variants[0];
  const total = (variant?.price ?? 0) * qty;

  const addToCart = () => {
    if (!variant) return;
    add(
      {
        variantId: variant.id,
        productId: product.id,
        productName: product.name,
        flowersCount: variant.quantity,
        price: variant.price,
        image: product.images[0]?.url ?? '',
      },
      qty,
    );
    haptic('success');
    navigate('/cart');
  };

  return (
    <div className="pb-28">
      <Header showBack={!tg()} />
      <Gallery images={product.images} alt={product.name} />

      <div className="p-4">
        <div className="flex items-start justify-between gap-3">
          <h1 className="text-xl font-bold leading-tight">{product.name}</h1>
          <span className="mt-0.5 whitespace-nowrap rounded-full border border-line px-2.5 py-1 text-xs text-muted">
            {product.category}
          </span>
        </div>
        {product.description && (
          <p className="mt-3 text-sm leading-relaxed text-muted">{product.description}</p>
        )}

        <h2 className="mb-2 mt-6 text-sm font-semibold uppercase tracking-wide text-muted">
          Размер букета
        </h2>
        <div className="space-y-2">
          {product.variants.map((v) => {
            const active = v.id === variant?.id;
            return (
              <button
                key={v.id}
                onClick={() => {
                  haptic('light');
                  setVariantId(v.id);
                }}
                className={`flex w-full items-center justify-between rounded-card border p-4 text-left transition-colors ${
                  active ? 'border-accent bg-accent/10' : 'border-line bg-surface'
                }`}
              >
                <span className="text-sm font-medium">{v.quantity} шт</span>
                <span className="text-base font-semibold">{formatPrice(v.price)}</span>
              </button>
            );
          })}
        </div>

        <div className="mt-6 flex items-center justify-between">
          <span className="text-sm font-semibold uppercase tracking-wide text-muted">
            Количество
          </span>
          <Stepper value={qty} onChange={(v) => (v < 1 ? undefined : setQty(v))} />
        </div>
      </div>

      <div className="pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 border-t border-line bg-page/95 px-4 pt-3 backdrop-blur">
        <button
          onClick={addToCart}
          disabled={!variant}
          className="w-full rounded-card bg-accent py-4 text-base font-semibold text-[#111111] shadow-card transition-transform active:scale-[0.98] disabled:opacity-50"
        >
          Добавить в корзину · {formatPrice(total)}
        </button>
      </div>
    </div>
  );
}
