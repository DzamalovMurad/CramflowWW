import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { ProductCard } from '../types';
import { discountPercent, formatPrice, inStock, isLowStock } from '../types';
import { content } from '../content';
import { haptic } from '../telegram';
import { useCart } from '../cart';
import { IconPlus, IconMinus, IconLeaf } from './icons';
import { hasSeasonBadge } from '../seasonal';

interface Props {
  product: ProductCard;
  index: number;
  onAdd: (product: ProductCard) => void | Promise<void>;
}

/**
 * Карточка каталога: фото со скелетоном, бейджи, цена со скидкой.
 * Кнопка «+» после добавления превращается в счётчик [− N +];
 * при первом добавлении фото «улетает» в корзину нижнего меню.
 */
export default function ProductCardView({ product, index, onAdd }: Props) {
  const off = discountPercent(product.price, product.old_price);
  const lowStock = isLowStock(product);
  const available = inStock(product);

  const { items, setQty } = useCart();
  const inCart = items.filter((i) => i.productId === product.id);
  const qty = inCart.reduce((s, i) => s + i.qty, 0);

  const [imgLoaded, setImgLoaded] = useState(false);
  const [busy, setBusy] = useState(false);
  const imgRef = useRef<HTMLImageElement | null>(null);

  // Fly-to-cart: клон фото улетает в иконку корзины нижнего меню.
  const flyToCart = () => {
    if (matchMedia('(prefers-reduced-motion: reduce)').matches) return;
    const img = imgRef.current;
    const target = document.querySelector('[data-cart-icon]');
    if (!img || !target) return;
    const a = img.getBoundingClientRect();
    const b = target.getBoundingClientRect();
    const clone = img.cloneNode(true) as HTMLImageElement;
    Object.assign(clone.style, {
      position: 'fixed',
      left: `${a.left}px`,
      top: `${a.top}px`,
      width: `${a.width}px`,
      height: `${a.height}px`,
      borderRadius: '20px',
      objectFit: 'cover',
      zIndex: '70',
      pointerEvents: 'none',
      margin: '0',
      opacity: '1',
    });
    document.body.appendChild(clone);
    const dx = b.left + b.width / 2 - (a.left + a.width / 2);
    const dy = b.top + b.height / 2 - (a.top + a.height / 2);
    clone
      .animate(
        [
          { transform: 'translate(0, 0) scale(1)', opacity: 1 },
          { transform: `translate(${dx}px, ${dy}px) scale(0.07)`, opacity: 0.35 },
        ],
        { duration: 600, easing: 'cubic-bezier(0.5, -0.05, 0.7, 1)' },
      )
      .addEventListener('finish', () => clone.remove());
  };

  const firstAdd = async () => {
    if (busy || !available) return;
    setBusy(true);
    haptic('medium');
    flyToCart();
    try {
      await onAdd(product);
    } finally {
      setBusy(false);
    }
  };

  // ± для уже добавленного товара: правим первый вариант этого товара в корзине.
  const bump = (d: number) => {
    const it = inCart[0];
    if (!it) return;
    haptic('light');
    setQty(it.variantId, it.qty + d);
  };

  return (
    <div className="animate-fade-up" style={{ animationDelay: `${Math.min(index * 40, 280)}ms` }}>
      <div className="relative">
        <Link
          to={`/product/${product.id}`}
          className="group block overflow-hidden rounded-card bg-tile shadow-card transition-transform duration-200 active:scale-[0.98]"
        >
          {/* aspect-[4/5] задан заранее — сетка не дёргается, когда грузятся фото */}
          <div className="relative aspect-[4/5] w-full overflow-hidden">
            {!imgLoaded && <div className="absolute inset-0 animate-pulse bg-tile" />}
            {product.image && (
              <img
                ref={(el) => {
                  imgRef.current = el;
                  // Фото из кэша не вызывает onLoad — проверяем состояние сами.
                  if (el?.complete && el.naturalWidth > 0 && !imgLoaded) setImgLoaded(true);
                }}
                src={product.image}
                alt={product.name}
                loading="lazy"
                decoding="async"
                onLoad={() => setImgLoaded(true)}
                className={`h-full w-full object-cover transition-all duration-500 group-active:scale-105 ${
                  imgLoaded ? 'opacity-100' : 'opacity-0'
                } ${available ? '' : 'grayscale'}`}
              />
            )}
            <div className="pointer-events-none absolute inset-x-0 bottom-0 h-[30%] bg-gradient-to-t from-black/60 to-transparent" />
            {!available && (
              <div className="absolute inset-0 flex items-center justify-center bg-page/55">
                <span className="rounded-full bg-ink px-3 py-1.5 text-[11px] font-bold lowercase text-page">
                  {content.catalog.soldOut}
                </span>
              </div>
            )}
          </div>
        </Link>

        {/* Бейджи слева сверху */}
        <div className="pointer-events-none absolute left-2.5 top-2.5 flex flex-col items-start gap-1.5">
          {seasonal && (
            <span className="badge badge-season">
              <IconLeaf size={11} />
              {content.seasonal.badge}
            </span>
          )}
          {product.is_hit && <span className="badge badge-hit">хит</span>}
          {off > 0 && <span className="badge badge-sale">−{off}%</span>}
          {lowStock && (
            <span className="badge badge-stock">
              {content.catalog.lastLeft} {product.stock}
            </span>
          )}
        </div>

        {/* «+» ⇄ счётчик. Кнопки 44px: промах здесь стоит лишнего букета в корзине. */}
        {available && (
          <div
            className={`fab-add absolute bottom-2.5 right-2.5 flex h-11 items-center overflow-hidden rounded-full transition-all duration-300 ease-in-out ${
              qty > 0 ? 'w-[124px]' : 'w-11'
            }`}
          >
            {qty > 0 ? (
              <div className="animate-fade-in flex w-full items-center">
                <button
                  type="button"
                  aria-label="убрать один"
                  onClick={() => bump(-1)}
                  className="flex h-11 w-11 flex-shrink-0 items-center justify-center active:opacity-60"
                >
                  <IconMinus size={16} />
                </button>
                <span className="flex-1 text-center font-mono text-[14px] font-bold tabular-nums">
                  {qty}
                </span>
                <button
                  type="button"
                  aria-label="добавить ещё один"
                  onClick={() => bump(1)}
                  className="flex h-11 w-11 flex-shrink-0 items-center justify-center active:opacity-60"
                >
                  <IconPlus size={16} />
                </button>
              </div>
            ) : (
              <button
                type="button"
                aria-label={`${content.catalog.addToCart}: ${product.name}`}
                onClick={firstAdd}
                disabled={busy}
                className="flex h-11 w-11 items-center justify-center"
              >
                <IconPlus size={20} />
              </button>
            )}
          </div>
        )}
      </div>

      <Link to={`/product/${product.id}`} className="mt-2.5 block">
        {available && (
          <p className="label mb-1 !text-[10px] text-accent-2">{content.catalog.deliveryToday}</p>
        )}
        <p className="line-clamp-1 text-[14px] font-bold leading-snug">{product.name}</p>
        <div className="mt-1 flex items-baseline gap-2">
          <span className="font-mono text-[16px] font-bold">{formatPrice(product.price)}</span>
          {off > 0 && (
            <span className="font-mono text-[13px] font-medium text-muted line-through">
              {formatPrice(product.old_price!)}
            </span>
          )}
        </div>
      </Link>
    </div>
  );
}
