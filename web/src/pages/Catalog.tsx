import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import Header from '../components/Header';
import CategoryChips from '../components/CategoryChips';
import FilterPills from '../components/FilterPills';
import ProductCardView from '../components/ProductCardView';
import { fetchProduct, fetchProducts } from '../api';
import { useCart } from '../cart';
import { haptic } from '../telegram';
import type { ProductCard } from '../types';

/** Каталог: категории + быстрые фильтры + грид карточек. */
export default function Catalog() {
  const [params, setParams] = useSearchParams();
  const category = params.get('category') ?? '';
  const filter = params.get('filter') ?? '';

  const [products, setProducts] = useState<ProductCard[] | null>(null);
  const [error, setError] = useState('');
  const { add } = useCart();

  useEffect(() => {
    let cancelled = false;
    setProducts(null);
    setError('');
    fetchProducts(category, filter)
      .then((list) => !cancelled && setProducts(list))
      .catch((e) => !cancelled && setError(e.message));
    return () => {
      cancelled = true;
    };
  }, [category, filter]);

  const updateParams = (key: 'category' | 'filter', value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  };

  // В корзину с карточки уходит самый доступный вариант букета.
  const addCheapest = async (card: ProductCard) => {
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

  return (
    <div className="pb-8">
      <Header />
      <div className="sticky top-14 z-10 border-b border-line bg-page/95 backdrop-blur">
        <CategoryChips selected={category} onSelect={(c) => updateParams('category', c)} />
        <FilterPills selected={filter} onSelect={(f) => updateParams('filter', f)} />
      </div>

      {error && <p className="p-6 text-center text-sm text-muted">{error}</p>}

      {products === null && !error && (
        <div className="grid grid-cols-2 gap-3 p-4">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="aspect-[3/4] animate-pulse rounded-card bg-line" />
          ))}
        </div>
      )}

      {products !== null && products.length === 0 && (
        <div className="p-10 text-center">
          <p className="text-4xl">🌷</p>
          <p className="mt-3 text-sm text-muted">
            По этим условиям букетов не нашлось.
            <br />
            Попробуйте изменить фильтры.
          </p>
        </div>
      )}

      {products !== null && products.length > 0 && (
        <div key={`${category}|${filter}`} className="grid grid-cols-2 gap-3 p-4">
          {products.map((p, i) => (
            <ProductCardView key={p.id} product={p} index={i} onAdd={addCheapest} />
          ))}
        </div>
      )}
    </div>
  );
}
