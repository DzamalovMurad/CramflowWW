import { useEffect, useMemo, useState } from "react";
import { MenuHeader } from "../components/Header";
import CategoryChips from "../components/CategoryChips";
import FilterPills from "../components/FilterPills";
import ProductCardView from "../components/ProductCardView";
import DealsRibbon, { pickDeals } from "../components/DealsRibbon";
import { Link } from "react-router-dom";
import {
  fetchAllProducts,
  fetchFreshToday,
  fetchMyOrders,
  fetchProduct,
  fetchProducts,
} from '../api';
import { useCart } from '../cart';
import { haptic } from '../telegram';
import { ACTIVE_STATUSES, content, statusLabels } from '../content';
import { formatDate, formatPrice, inStock, type Order, type ProductCard } from '../types';
import { IconSort } from '../components/icons';
import { isSeasonActive, isSeasonPick, seasonPicks } from '../seasonal';

const c = content.home;

type Sort = "" | "cheap" | "expensive";

/**
 * Главная = меню (скелет Bunch): лента «подешевле» → sticky-фильтры → сетка →
 * горизонтальная полка WOW → шторка сортировки. Букеты видны сразу после загрузки.
 */
export default function Home() {
  const [category, setCategory] = useState("");
  const [filter, setFilter] = useState("");
  const [sort, setSort] = useState<Sort>("");
  const [sheetOpen, setSheetOpen] = useState(false);
  // Сезонная подборка «к 1 сентября» — отбор в браузере, запрос к API не меняется.
  const [season, setSeason] = useState(false);
  const seasonOn = isSeasonActive();

  const [products, setProducts] = useState<ProductCard[] | null>(null);
  const [error, setError] = useState("");
  const [freshToday, setFreshToday] = useState<string | null>(null);
  const [activeOrder, setActiveOrder] = useState<Order | null>(null);
  const { add } = useCart();

  // Без фильтров берём прогретый лоадером кэш — сетка появляется мгновенно.
  useEffect(() => {
    let cancelled = false;
    setProducts(null);
    setError("");
    const load =
      category || filter ? fetchProducts(category, filter) : fetchAllProducts();
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

  // Состояние активного заказа должно быть видно сразу, без вопросов менеджеру.
  useEffect(() => {
    fetchMyOrders()
      .then((list) =>
        setActiveOrder(
          list.find((o) => ACTIVE_STATUSES.includes(o.status)) ?? null,
        ),
      )
      .catch(() => {});
  }, []);

  // Лента «подешевле»: отдельный запрос за популярным, чтобы отбор не зависел
  // от текущей категории и фильтра — под шапкой всегда одно и то же.
  const [deals, setDeals] = useState<ProductCard[]>([]);
  useEffect(() => {
    fetchProducts('', 'popular')
      .then((list) => setDeals(pickDeals(list)))
      .catch(() => {}); // лента необязательна — без неё витрина работает
  }, []);

  const addCheapest = async (card: ProductCard) => {
    const product = await fetchProduct(card.id).catch(() => null);
    const variant = product?.variants[0];
    if (!product || !variant || !inStock(product)) return;
    add({
      variantId: variant.id,
      productId: product.id,
      productName: product.name,
      flowersCount: variant.quantity,
      price: variant.price,
      image: product.images[0]?.url ?? "",
    });
    haptic("success");
  };

  const sorted = useMemo(() => {
    if (!products) return null;
    if (sort === "cheap")
      return [...products].sort((a, b) => a.price - b.price);
    if (sort === "expensive")
      return [...products].sort((a, b) => b.price - a.price);
    return products;
  }, [products, sort]);

  // Полка WOW показывается только на «чистой» витрине (без фильтров).
  const shelf = useMemo(() => {
    if (season || category || filter || !products) return [];
    return products.filter((p) => p.category === 'WOW');
  }, [products, category, filter, season]);
  const grid = useMemo(() => {
    if (!sorted) return null;
    const base = season ? sorted.filter(isSeasonPick) : sorted;
    if (shelf.length === 0) return base;
    return base.filter((p) => p.category !== 'WOW');
  }, [sorted, shelf, season]);

  // Таблетку показываем, только если ей есть что открыть, — иначе тап ведёт
  // в пустую витрину. Уже включённую не прячем, чтобы её можно было выключить.
  const seasonReady = useMemo(
    () => season || (!!products && seasonPicks(products).length > 0),
    [products, season],
  );

  const minPrice =
    grid && grid.length > 0 ? Math.min(...grid.map((p) => p.price)) : 0;

  return (
    <div className="pb-24">
      <MenuHeader />

      {/* АКТИВНЫЙ ЗАКАЗ — статус на виду, без вопросов менеджеру */}
      {activeOrder && (
        <Link
          to="/profile"
          className="animate-fade-in mx-4 mt-3 flex min-h-[56px] items-center justify-between gap-3 rounded-card border border-accent/40 bg-accent/10 px-4 py-3"
        >
          <span className="min-w-0">
            <span className="label block !text-[10px]">
              {c.activeOrder} #{activeOrder.id}
            </span>
            <span className="mt-0.5 block truncate text-[15px] font-medium">
              {statusLabels[activeOrder.status] ?? activeOrder.status_label}
            </span>
          </span>
          <span className="whitespace-nowrap text-[12px] lowercase text-muted">
            {formatDate(activeOrder.delivery_date)}, {activeOrder.delivery_time}
          </span>
        </Link>
      )}

      {/* «СЕГОДНЯ НА БАЗЕ» — живая строка о том, что закупил флорист */}
      {freshToday && (
        <p className="mx-4 mt-3 flex items-center gap-2 rounded-input border border-line bg-surface px-3.5 py-2.5 text-[13px] leading-snug">
          <span className="relative flex h-2 w-2 flex-shrink-0">
            <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent opacity-70" />
            <span className="relative inline-flex h-2 w-2 rounded-full bg-accent" />
          </span>
          <span className="label !text-[10px]">{c.freshToday}</span>
          <span className="min-w-0 flex-1 truncate text-muted">{freshToday}</span>
        </p>
      )}

      {/* ЛЕНТА «ПОДЕШЕВЛЕ» — ходовые букеты едут сами, до первого скролла */}
      {deals.length > 0 && <DealsRibbon items={deals} />}

      {/* ФИЛЬТРЫ — прилипают к верху; фон сплошной, карточки уходят под него без «грязи» */}
      <div className="sticky top-0 z-10 mt-2 border-b border-line bg-page">
        <CategoryChips selected={category} onSelect={setCategory} />
        <div className="flex items-center gap-1 pr-2">
          <div className="min-w-0 flex-1">
            <FilterPills
              selected={filter}
              onSelect={setFilter}
              seasonal={seasonOn && seasonReady ? { active: season, onToggle: setSeason } : undefined}
            />
          </div>
          <button
            type="button"
            aria-label={content.sort.title}
            onClick={() => {
              haptic("light");
              setSheetOpen(true);
            }}
            className={`mb-2 flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full border ${
              sort
                ? "border-transparent bg-accent text-on-accent"
                : "border-line bg-surface text-ink"
            }`}
          >
            <IconSort size={16} />
          </button>
        </div>
      </div>

      {/* СЕТКА ТОВАРОВ */}
      {grid !== null && grid.length > 0 && (
        <div className="flex items-baseline justify-between px-4 pb-2 pt-7">
          <h2 className="heading">{c.sectionTitle}</h2>
          <p className="label !text-[11px]">
            {grid.length} {content.catalog.count} · {content.catalog.priceFrom}{" "}
            {formatPrice(minPrice)}
          </p>
        </div>
      )}

      {error && (
        <p className="p-6 text-center text-sm lowercase text-muted">{error}</p>
      )}

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
        <div
          key={`${category}|${filter}|${sort}`}
          className="grid grid-cols-2 gap-x-3 gap-y-6 p-4"
        >
          {grid.map((p, i) => (
            <ProductCardView
              key={p.id}
              product={p}
              index={i}
              onAdd={addCheapest}
            />
          ))}
        </div>
      )}

      {/* ГОРИЗОНТАЛЬНАЯ ПОЛКА WOW */}
      {shelf.length > 0 && (
        <section className="pt-2">
          <div className="flex items-baseline justify-between px-4">
            <h2 className="heading">{c.shelfTitle}</h2>
            <button
              type="button"
              onClick={() => {
                haptic("light");
                setCategory("WOW");
                window.scrollTo({ top: 0, behavior: "smooth" });
              }}
              className="-mr-2 flex min-h-[44px] items-center px-2 text-[13px] font-semibold lowercase text-accent-2"
            >
              {c.shelfAll} {shelf.length} →
            </button>
          </div>
          <p className="mt-1 px-4 text-[12px] lowercase text-muted">
            {c.shelfCaption}
          </p>
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
          <div
            className="sheet-overlay animate-fade-in"
            onClick={() => setSheetOpen(false)}
          />
          <div className="sheet">
            <div className="sheet-handle" />
            <h3 className="heading mb-3">{content.sort.title}</h3>
            {(
              [
                ["", content.sort.default],
                ["cheap", content.sort.cheap],
                ["expensive", content.sort.expensive],
              ] as [Sort, string][]
            ).map(([value, label]) => {
              const active = sort === value;
              return (
                <button
                  key={value || "default"}
                  type="button"
                  onClick={() => {
                    haptic("light");
                    setSort(value);
                    setSheetOpen(false);
                  }}
                  className="flex min-h-[52px] w-full items-center justify-between"
                >
                  <span
                    className={`text-[15px] lowercase ${active ? "font-semibold" : "text-muted"}`}
                  >
                    {label}
                  </span>
                  <span
                    className={`flex h-5 w-5 items-center justify-center rounded-full border-2 ${
                      active ? "border-accent" : "border-line"
                    }`}
                  >
                    {active && (
                      <span className="h-2.5 w-2.5 rounded-full bg-accent" />
                    )}
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
