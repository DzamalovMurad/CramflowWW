import { useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import type { ProductCard } from '../types';
import { formatPrice, discountPercent } from '../types';
import { content } from '../content';
import { haptic } from '../telegram';
import { useCart } from '../cart';
import { IconPlus, IconMinus } from './icons';

interface Props {
  product: ProductCard;
  index: number;
  onAdd: (product: ProductCard) => void | Promise<void>;
}

/**
 * Карточка каталога (стиль Bunch): фото со скелетоном, бейджи, цена со скидкой.
 * Кнопка «+» после добавления плавно расширяется в счётчик [−  N  +];
 * при первом добавлении фото «улетает» в корзину нижнего меню.
 */
export default function ProductCardView({ product, index, onAdd }: Props) {
  const off = discountPercent(product.price, product.old_price);
  const lowStock = product.stock !== undefined && product.stock > 0 && product.stock <= 5;

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
    if (busy) return;
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
    <div
      className="animate-fade-up"
      style={{ animationDelay: `${Math.min(index * 40, 280)}ms` }}
    >
      <div className="relative">
        <Link
          to={`/product/${product.id}`}
          className="group block overflow-hidden rounded-card bg-tile shadow-card transition-transform duration-200 active:scale-[0.98]"
        >
          <div className="relative aspect-[4/5] w-full overflow-hidden">
            {/* Скелетон, пока фото не загрузилось */}
            {!imgLoaded && <div className="absolute inset-0 animate-pulse bg-tile" />}
            {product.image && (
              <img
                ref={(el) => {
                  imgRef.current = el;
                  if (el?.complete && el.naturalWidth > 0 && !imgLoaded) setImgLoaded(true);
                }}
                src={product.image}
                alt={product.name}
                loading="lazy"
                onLoad={() => setImgLoaded(true)}
                className={`h-full w-full object-cover transition-all duration-500 group-active:scale-105 ${
                  imgLoaded ? 'opacity-100' : 'opacity-0'
                }`}
              />
            )}
          </div>
        </Link>

        {/* Бейджи слева сверху */}
        <div className="pointer-events-none absolute left-2.5 top-2.5 flex flex-col items-start gap-1.5">
          {product.is_hit && <span className="badge badge-hit">хит</span>}
          {off > 0 && <span className="badge badge-sale">−{off}%</span>}
          {lowStock && <span className="badge badge-stock">осталось {product.stock}</span>}
        </div>

        {/* «+» ⇄ счётчик: контейнер плавно меняет ширину */}
        <div
          className={`fab-add absolute bottom-2.5 right-2.5 flex h-10 items-center overflow-hidden rounded-full transition-all duration-300 ease-in-out ${
            qty > 0 ? 'w-[112px]' : 'w-10'
          }`}
        >
          {qty > 0 ? (
            <div className="animate-fade-in flex w-full items-center">
              <button
                type="button"
                aria-label="убрать"
                onClick={() => bump(-1)}
                className="flex h-10 w-9 flex-shrink-0 items-center justify-center active:opacity-60"
              >
                <IconMinus size={16} />
              </button>
              <span className="flex-1 text-center text-[14px] font-extrabold tabular-nums">
                {qty}
              </span>
              <button
                type="button"
                aria-label="добавить"
                onClick={() => bump(1)}
                className="flex h-10 w-9 flex-shrink-0 items-center justify-center active:opacity-60"
              >
                <IconPlus size={16} />
              </button>
            </div>
          ) : (
            <button
              type="button"
              aria-label={content.catalog.addToCart}
              onClick={firstAdd}
              className="flex h-10 w-10 items-center justify-center"
            >
              <IconPlus size={20} />
            </button>
          )}
        </div>
      </div>

      <Link to={`/product/${product.id}`} className="mt-2.5 block">
        <p className="label mb-1 !text-[10px] text-accent-2">{content.catalog.deliveryToday}</p>
        <p className="line-clamp-1 text-[14px] font-bold leading-snug">{product.name}</p>
        <div className="mt-1 flex items-baseline gap-2">
          <span className="text-[16px] font-extrabold">{formatPrice(product.price)}</span>
          {off > 0 && (
            <span className="text-[13px] font-medium text-muted line-through">
              {formatPrice(product.old_price!)}
            </span>
          )}
        </div>
      </Link>
    </div>
  );
}
