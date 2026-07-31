/**
 * Скелетоны загрузки. Один источник правды: сетка каталога и карточка товара
 * должны «мигать» одинаково, где бы ни грузились — на главной, в каталоге,
 * в подборке хитов. Белый экран не показываем нигде.
 *
 * Размеры повторяют реальные блоки (aspect-[4/5] у фото, те же отступы),
 * иначе при появлении данных страница дёргается.
 */

/** Одна карточка каталога: фото + название + цена. */
export function ProductCardSkeleton() {
  return (
    <div aria-hidden>
      <div className="aspect-[4/5] animate-pulse rounded-card bg-tile" />
      <div className="mt-2.5 h-3.5 w-2/3 animate-pulse rounded bg-tile" />
      <div className="mt-2 h-4 w-1/3 animate-pulse rounded bg-tile" />
    </div>
  );
}

/** Сетка каталога на время загрузки. */
export function CatalogGridSkeleton({ count = 4 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 gap-x-3 gap-y-6 p-4" role="status" aria-label="загружаем букеты">
      {Array.from({ length: count }).map((_, i) => (
        <ProductCardSkeleton key={i} />
      ))}
    </div>
  );
}

/** Горизонтальная полка (wow-букеты, хиты в корзине). */
export function ShelfSkeleton({ count = 3 }: { count?: number }) {
  return (
    <div className="no-scrollbar flex gap-3 overflow-x-hidden px-4" role="status" aria-label="загружаем подборку">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="w-[150px] flex-shrink-0">
          <ProductCardSkeleton />
        </div>
      ))}
    </div>
  );
}

/** Карточка товара: галерея, заголовок, описание, чипы вариантов. */
export function ProductPageSkeleton() {
  return (
    <div role="status" aria-label="загружаем букет">
      <div className="aspect-square w-full animate-pulse bg-tile" />
      <div className="p-5">
        <div className="h-3 w-20 animate-pulse rounded bg-tile" />
        <div className="mt-2 h-7 w-2/3 animate-pulse rounded bg-tile" />
        <div className="mt-3 space-y-2">
          <div className="h-3.5 w-full animate-pulse rounded bg-tile" />
          <div className="h-3.5 w-4/5 animate-pulse rounded bg-tile" />
        </div>
        <div className="mt-7 h-3 w-24 animate-pulse rounded bg-tile" />
        <div className="mt-2.5 flex gap-2">
          {[96, 110, 88].map((w) => (
            <div key={w} className="h-11 animate-pulse rounded-button bg-tile" style={{ width: w }} />
          ))}
        </div>
      </div>
    </div>
  );
}
