import { Link } from 'react-router-dom';
import type { ProductCard } from '../types';
import { formatPrice } from '../types';
import { content, categoryLabels } from '../content';
import { haptic } from '../telegram';

interface Props {
  product: ProductCard;
  index: number;
  onAdd: (product: ProductCard) => void;
}

/** Карточка каталога: фото на серой плитке, название, цена, тихая кнопка «в корзину». */
export default function ProductCardView({ product, index, onAdd }: Props) {
  return (
    <div
      className="animate-fade-up"
      style={{ animationDelay: `${Math.min(index * 45, 300)}ms` }}
    >
      <Link to={`/product/${product.id}`} className="block">
        <div className="aspect-[4/5] w-full overflow-hidden rounded-card bg-tile">
          {product.image && (
            <img
              src={product.image}
              alt={product.name}
              loading="lazy"
              className="h-full w-full object-cover"
            />
          )}
        </div>
        <p className="mt-2.5 line-clamp-1 text-[13px] font-medium lowercase leading-snug">
          {product.name}
        </p>
        <p className="text-xs lowercase text-muted">{categoryLabels[product.category] ?? product.category}</p>
        <p className="mt-1 text-[15px] font-bold">
          {content.catalog.priceFrom} {formatPrice(product.price)}
        </p>
      </Link>
      <button
        onClick={() => {
          haptic('medium');
          onAdd(product);
        }}
        className="mt-2 w-full rounded-button bg-tile py-2.5 text-[13px] font-medium lowercase text-ink transition-transform active:scale-[0.97]"
      >
        {content.catalog.addToCart}
      </button>
    </div>
  );
}
