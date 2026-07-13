import { Link } from 'react-router-dom';
import type { ProductCard } from '../types';
import { formatPrice } from '../types';
import { haptic } from '../telegram';

interface Props {
  product: ProductCard;
  index: number;
  onAdd: (product: ProductCard) => void;
}

/** Карточка каталога: фото 1:1, название, цена «от …», кнопка в корзину. */
export default function ProductCardView({ product, index, onAdd }: Props) {
  return (
    <div
      className="animate-fade-up overflow-hidden rounded-card border border-line bg-surface shadow-card"
      style={{ animationDelay: `${Math.min(index * 45, 300)}ms` }}
    >
      <Link to={`/product/${product.id}`} className="block">
        <div className="aspect-square w-full overflow-hidden bg-line">
          {product.image && (
            <img
              src={product.image}
              alt={product.name}
              loading="lazy"
              className="h-full w-full object-cover"
            />
          )}
        </div>
        <div className="px-3 pt-3">
          <h3 className="line-clamp-2 min-h-10 text-sm font-medium leading-5">{product.name}</h3>
          <p className="mt-1 text-base font-semibold">от {formatPrice(product.price)}</p>
        </div>
      </Link>
      <div className="p-3">
        <button
          onClick={() => {
            haptic('medium');
            onAdd(product);
          }}
          className="w-full rounded-card bg-accent py-2 text-sm font-semibold text-[#111111] transition-transform active:scale-95"
        >
          Добавить в корзину
        </button>
      </div>
    </div>
  );
}
