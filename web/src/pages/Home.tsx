import { useEffect, useMemo, useRef, useState } from 'react';
import { MenuHeader } from '../components/Header';
import CategoryChips from '../components/CategoryChips';
import FilterPills from '../components/FilterPills';
import ProductCardView from '../components/ProductCardView';
import { fetchAllProducts, fetchProduct, fetchProducts, fetchFreshToday } from '../api';
import { useCart } from '../cart';
import { haptic } from '../telegram';
import { content } from '../content';
import { formatPrice, type ProductCard } from '../types';
import { IconSort } from '../components/icons';

const c = content.home;

type Sort = '' | 'cheap' | 'expensive';

/**
 * Главная = меню (скелет Bunch): баннеры → sticky-фильтры → сетка товаров →
 * горизонтальная полка WOW → шторка сортировки. Букеты видны сразу после загрузки.
 */
export default function Home() {
  const [category, setCategory] = useState('');
  const [filter, setFilter] = useState('');
  const [sort, setSort] = useState<Sort>('');
  const [sheetOpen, setSheetOpen] = useState(false);

  const [products, setProducts] = useState<ProductCard[] | null>(null);
  const [error, setError] = useState('');
  const [freshToday, setFreshToday] = useState<string | null>(null);
  const { add } = useCart();

  // Без фильтров берём прогретый лоадером кэш — сетка появляется мгновенно.
  useEffect(() => {
    let cancelled = false;
    setProducts(null);
    setError('');
    const load = category || filter ? fetchProducts(category, filter) : fetchAllProducts();
    load
      .then((list) => !cancelled && setProducts(list))
      .catch((e) => !cancelled && setError(e.message));
    return () => {
      cancelled = true;
    };
  }, [category, filter]);

  useEffect(() => {
    fetchFreshToday()
      .then((data) => data.items && setFreshToday(data.items))
      .catch(() => {});
  }, []);

  // Баннеры: следим за скроллом карусели для точек-индикаторов.
  const bannerRef = useRef<HTMLDivElement>(null);
  const [bannerIdx, setBannerIdx] = useState(0);
  const onBannerScroll = () => {
    const el = bannerRef.current;
    if (el) setBannerIdx(Math.round(el.scrollLeft / el.clientWidth));
  };

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

  const sorted = useMemo(() => {
    if (!products) return null;
    if (sort === 'cheap') return [...products].sort((a, b) => a.price - b.price);
    if (sort === 'expensive') return [...products].sort((a, b) => b.price - a.price);
    return products;
  }, [products, sort]);

  // Полка WOW показывается только на «чистой» витрине (без фильтров).
  const shelf = useMemo(() => {
    if (category || filter || !products) return [];
    return products.filter((p) => p.category === 'WOW');
  }, [products, category, filter]);
  const grid = useMemo(() => {
    if (!sorted) return null;
    if (shelf.length === 0) return sorted;
    return sorted.filter((p) => p.category !== 'WOW');
  }, [sorted, shelf]);

  const minPrice = grid && grid.length > 0 ? Math.min(...grid.map((p) => p.price)) : 0;
  const banners = 1 + (freshToday ? 1 : 0);

  return (
    <div className="pb-24">
      <MenuHeader />

      {/* БАННЕРЫ */}
      <section className="px-4 pt-3">
        <div ref={bannerRef} className="banner-scroll" onScroll={onBannerScroll}>
          <div className="banner-card relative bg-ink p-5">
            <p className="label mb-2 !text-accent-ink">{c.badge}</p>
            <h1 className="display text-[26px] text-page">{c.title}</h1>
            <p className="mt-1.5 text-[12px] lowercase text-page/70">{c.subtitle}</p>
            <span className="absolute right-4 top-4 h-2.5 w-2.5 rounded-full bg-accent" />
          </div>
          {freshToday && (
            <div className="banner-card relative border border-line bg-surface p-5">
              <p className="label mb-2 flex items-center gap-2">
                <span className="relative flex h-2 w-2">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent opacity-70" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-accent" />
                </span>
                {c.freshToday}
              </p>
              <p className="display text-[20px]">{freshToday}</p>
            </div>
          )}
        </div>
        {banners > 1 && (
          <div className="mt-2 flex justify-center gap-1.5">
            {Array.from({ length: banners }).map((_, i) => (
              <span
                key={i}
                className={`h-1.5 rounded-full transition-all ${
                  i === bannerIdx ? 'w-4 bg-ink' : 'w-1.5 bg-line'
                }`}
              />
            ))}
          </div>
        )}
      </section>

      {/* ФИЛЬТРЫ — прилипают к верху; фон сплошной, карточки уходят под него без «грязи» */}
      <div className="sticky top-0 z-10 mt-2 border-b border-line bg-page">
        <CategoryChips selected={category} onSelect={setCategory} />
        <div className="flex items-center gap-1 pr-2">
          <div className="min-w-0 flex-1">
            <FilterPills selected={filter} onSelect={setFilter} />
          </div>
          <button
            type="button"
            aria-label={content.sort.title}
            onClick={() => {
              haptic('light');
              setSheetOpen(true);
            }}
            className={`mb-3 flex h-9 w-9 flex-shrink-0 items-center justify-center rounded-full border ${
              sort ? 'border-transparent bg-accent text-on-accent' : 'border-line bg-surface text-ink'
            }`}
          >
            <IconSort size={16} />
          </button>
        </div>
      </div>

      {/* СЕТКА ТОВАРОВ */}
      {grid !== null && grid.length > 0 && (
        <div className="flex items-baseline justify-between px-4 pb-1 pt-4">
          <h2 className="display text-[22px]">{c.sectionTitle}</h2>
          <p className="label !text-[11px]">
            {grid.length} {content.catalog.count} · {content.catalog.priceFrom} {formatPrice(minPrice)}
          </p>
        </div>
      )}

      {error && <p className="p-6 text-center text-sm lowercase text-muted">{error}</p>}

      {grid === null && !error && (
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

      {grid !== null && grid.length === 0 && (
        <p className="px-10 py-16 text-center text-sm lowercase leading-relaxed text-muted">
          {content.catalog.empty}
        </p>
      )}

      {grid !== null && grid.length > 0 && (
        <div key={`${category}|${filter}|${sort}`} className="grid grid-cols-2 gap-x-3 gap-y-6 p-4">
          {grid.map((p, i) => (
            <ProductCardView key={p.id} product={p} index={i} onAdd={addCheapest} />
          ))}
        </div>
      )}

      {/* ГОРИЗОНТАЛЬНАЯ ПОЛКА WOW */}
      {shelf.length > 0 && (
        <section className="pt-2">
          <div className="flex items-baseline justify-between px-4">
            <h2 className="display text-[22px]">{c.shelfTitle}</h2>
            <button
              type="button"
              onClick={() => {
                haptic('light');
                setCategory('WOW');
                window.scrollTo({ top: 0, behavior: 'smooth' });
              }}
              className="text-[13px] font-bold lowercase text-accent-2"
            >
              {c.shelfAll} {shelf.length} →
            </button>
          </div>
          <p className="mt-1 px-4 text-[12px] lowercase text-muted">{c.shelfCaption}</p>
          <div className="no-scrollbar mt-3 flex gap-3 overflow-x-auto px-4">
            {shelf.map((p, i) => (
              <div key={p.id} className="w-[210px] flex-shrink-0">
                <ProductCardView product={p} index={i} onAdd={addCheapest} />
              </div>
            ))}
          </div>
        </section>
      )}

      {/* ШТОРКА СОРТИРОВКИ */}
      {sheetOpen && (
        <>
          <div className="sheet-overlay animate-fade-in" onClick={() => setSheetOpen(false)} />
          <div className="sheet">
            <div className="sheet-handle" />
            <h3 className="display mb-3 text-[19px]">{content.sort.title}</h3>
            {(
              [
                ['', content.sort.default],
                ['cheap', content.sort.cheap],
                ['expensive', content.sort.expensive],
              ] as [Sort, string][]
            ).map(([value, label]) => {
              const active = sort === value;
              return (
                <button
                  key={value || 'default'}
                  type="button"
                  onClick={() => {
                    haptic('light');
                    setSort(value);
                    setSheetOpen(false);
                  }}
                  className="flex w-full items-center justify-between py-3.5"
                >
                  <span className={`text-[15px] lowercase ${active ? 'font-bold' : 'text-muted'}`}>
                    {label}
                  </span>
                  <span
                    className={`flex h-5 w-5 items-center justify-center rounded-full border-2 ${
                      active ? 'border-accent' : 'border-line'
                    }`}
                  >
                    {active && <span className="h-2.5 w-2.5 rounded-full bg-accent" />}
                  </span>
                </button>
              );
            })}
          </div>
        </>
      )}
    </div>
  );
}
