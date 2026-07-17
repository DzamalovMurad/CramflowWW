import { Link } from 'react-router-dom';
import type { ProductCard } from '../types';
import { formatPrice, discountPercent } from '../types';
import { content } from '../content';
import { haptic } from '../telegram';
import { IconPlus } from './icons';

interface Props {
  product: ProductCard;
  index: number;
  onAdd: (product: ProductCard) => void;
}

/** Карточка каталога в стиле Bunch: фото с бейджами, плавающая «+», цена со скидкой. */
export default function ProductCardView({ product, index, onAdd }: Props) {
  const off = discountPercent(product.price, product.old_price);
  const lowStock = product.stock !== undefined && product.stock > 0 && product.stock <= 5;

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
          <div className="aspect-[4/5] w-full overflow-hidden">
            {product.image && (
              <img
                src={product.image}
                alt={product.name}
                loading="lazy"
                className="h-full w-full object-cover transition-transform duration-500 group-active:scale-105"
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

        {/* Плавающая круглая «+» — добавить в корзину в один тап */}
        <button
          aria-label={content.catalog.addToCart}
          onClick={() => {
            haptic('medium');
            onAdd(product);
          }}
          className="fab-add absolute bottom-2.5 right-2.5 flex h-10 w-10 items-center justify-center rounded-full"
        >
          <IconPlus size={20} />
        </button>
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
