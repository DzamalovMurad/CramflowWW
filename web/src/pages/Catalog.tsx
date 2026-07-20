import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import Header from '../components/Header';
import CategoryChips from '../components/CategoryChips';
import FilterPills from '../components/FilterPills';
import ProductCardView from '../components/ProductCardView';
import { fetchProduct, fetchProducts } from '../api';
import { useCart } from '../cart';
import { haptic } from '../telegram';
import { content } from '../content';
import { formatPrice, type ProductCard } from '../types';

/** Каталог: поиск + категории + быстрые фильтры + editorial-сетка. */
export default function Catalog() {
  const [params, setParams] = useSearchParams();
  const category = params.get('category') ?? '';
  const filter = params.get('filter') ?? '';

  // Поиск: локальный ввод мгновенный, запрос — с debounce 300мс.
  const [query, setQuery] = useState('');
  const [search, setSearch] = useState('');
  useEffect(() => {
    const t = setTimeout(() => setSearch(query.trim()), 300);
    return () => clearTimeout(t);
  }, [query]);

  const [products, setProducts] = useState<ProductCard[] | null>(null);
  const [error, setError] = useState('');
  const { add } = useCart();

  useEffect(() => {
    let cancelled = false;
    setProducts(null);
    setError('');
    fetchProducts(category, filter, search)
      .then((list) => !cancelled && setProducts(list))
      .catch((e) => !cancelled && setError(e.message));
    return () => {
      cancelled = true;
    };
  }, [category, filter, search]);

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

  const minPrice = products && products.length > 0 ? Math.min(...products.map((p) => p.price)) : 0;

  return (
    <div className="pb-24">
      <Header search={{ value: query, onChange: setQuery }} />
      <div className="sticky top-14 z-10 border-b border-line bg-page">
        <CategoryChips selected={category} onSelect={(c) => updateParams('category', c)} />
        <FilterPills selected={filter} onSelect={(f) => updateParams('filter', f)} />
      </div>

      {products !== null && products.length > 0 && (
        <div className="flex items-baseline justify-between px-4 pb-1 pt-4">
          <h2 className="display text-[22px]">{content.catalog.title}</h2>
          <p className="label !text-[11px]">
            {products.length} {content.catalog.count} · {content.catalog.priceFrom} {formatPrice(minPrice)}
          </p>
        </div>
      )}

      {error && <p className="p-6 text-center text-sm lowercase text-muted">{error}</p>}

      {products === null && !error && (
        <div className="grid grid-cols-2 gap-x-3 gap-y-6 p-4">
          {[...Array(4)].map((_, i) => (
            <div key={i}>
              <div className="aspect-[4/5] animate-pulse rounded-card bg-tile" />
              <div className="mt-2.5 h-3.5 w-2/3 animate-pulse rounded bg-tile" />
              <div className="mt-2 h-4 w-1/3 animate-pulse rounded bg-tile" />
            </div>
          ))}
        </div>
      )}

      {products !== null && products.length === 0 && (
        <p className="px-10 py-16 text-center text-sm lowercase leading-relaxed text-muted">
          {search ? content.catalog.nothingFound : content.catalog.empty}
        </p>
      )}

      {products !== null && products.length > 0 && (
        <div key={`${category}|${filter}|${search}`} className="grid grid-cols-2 gap-x-3 gap-y-6 p-4">
          {products.map((p, i) => (
            <ProductCardView key={p.id} product={p} index={i} onAdd={addCheapest} />
          ))}
        </div>
      )}
    </div>
  );
}
