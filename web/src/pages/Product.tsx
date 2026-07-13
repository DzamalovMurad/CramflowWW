import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import Header from '../components/Header';
import Gallery from '../components/Gallery';
import Stepper from '../components/Stepper';
import { fetchProduct } from '../api';
import { useCart } from '../cart';
import { haptic, tg } from '../telegram';
import { content, categoryLabels } from '../content';
import { formatPrice, type Product } from '../types';

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
        <p className="p-10 text-center text-sm lowercase text-muted">{error}</p>
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
    <div className="pb-32">
      <Header showBack={!tg()} />
      <Gallery images={product.images} alt={product.name} />

      <div className="p-5">
        <p className="label">{categoryLabels[product.category] ?? product.category}</p>
        <h1 className="display mt-1 text-[26px]">{product.name}</h1>
        {product.description && (
          <p className="mt-3 text-sm leading-relaxed text-muted">{product.description}</p>
        )}
        <p className="mt-4 border-l-2 border-accent/30 pl-3 text-xs leading-relaxed text-muted">
          {content.product.availabilityNote}
        </p>

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
                className={`rounded-button border px-4 py-2.5 text-sm font-medium transition-colors ${
                  active ? 'border-ink bg-ink text-page' : 'border-line bg-page text-ink'
                }`}
              >
                {v.quantity} {content.product.flowersUnit} · {formatPrice(v.price)}
              </button>
            );
          })}
        </div>

        <div className="mt-7 flex items-center justify-between">
          <p className="label">{content.product.qtyLabel}</p>
          <Stepper value={qty} onChange={(v) => (v < 1 ? undefined : setQty(v))} />
        </div>
      </div>

      <div className="pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 border-t border-line bg-page/95 px-4 pt-3 backdrop-blur">
        <button
          onClick={addToCart}
          disabled={!variant}
          className="flex w-full items-center justify-between rounded-button bg-accent px-5 py-4 text-[15px] font-bold lowercase text-on-accent transition-transform active:scale-[0.98] disabled:opacity-50"
        >
          <span>{content.product.addToCart}</span>
          <span>{formatPrice(total)}</span>
        </button>
      </div>
    </div>
  );
}
