import { Link } from 'react-router-dom';
import { content } from '../content';
import { formatPrice, inStock, type ProductCard } from '../types';

/**
 * Лента «в наличии подешевле»: самые ходовые букеты из нижней части прайса
 * едут сами по себе под шапкой — витрина живёт до того, как человек начал
 * листать каталог.
 *
 * Лента едет за счёт CSS-анимации трека, а не скролла: список отрисован
 * дважды, трек сдвигается ровно на половину своей ширины и возвращается
 * в начало — стык незаметен. Палец останавливает движение (:active),
 * при prefers-reduced-motion анимации нет вовсе — см. .ribbon-track.
 */
export default function DealsRibbon({ items }: { items: ProductCard[] }) {
  // Дублируем список — вторая копия закрывает «хвост» кадра при сдвиге.
  const loop = [...items, ...items];

  return (
    <section className="pt-3" aria-label={content.home.dealsTitle}>
      <div className="mb-2 flex items-baseline justify-between px-4">
        <h2 className="heading">{content.home.dealsTitle}</h2>
        <span className="label !text-[10px]">{content.home.dealsHint}</span>
      </div>

      <div className="ribbon-mask">
        <div className="ribbon-track">
          {loop.map((p, i) => (
            <Link
              key={`${p.id}-${i}`}
              to={`/product/${p.id}`}
              // Вторая копия — декорация: скринридер не должен читать список дважды.
              aria-hidden={i >= items.length}
              tabIndex={i >= items.length ? -1 : undefined}
              className="ribbon-item"
            >
              <span className="h-14 w-14 flex-shrink-0 overflow-hidden rounded-[14px] bg-tile">
                {p.image && (
                  <img
                    src={p.image}
                    alt=""
                    width={56}
                    height={56}
                    loading="lazy"
                    className="h-full w-full object-cover"
                  />
                )}
              </span>
              <span className="min-w-0">
                <span className="card-title block max-w-[132px] truncate text-[13px] leading-tight">
                  {p.name}
                </span>
                <span className="price mt-1 block text-[14px]">{formatPrice(p.price)}</span>
              </span>
            </Link>
          ))}
        </div>
      </div>
    </section>
  );
}

/** Отбор для ленты: в наличии, из самых ходовых — и подешевле. */
export function pickDeals(popular: ProductCard[], limit = 10): ProductCard[] {
  return [...popular.filter(inStock)]
    .slice(0, 20) // «ходовые»: верх списка популярного
    .sort((a, b) => a.price - b.price) // «дешёвые»: из них — самые доступные
    .slice(0, limit);
}
