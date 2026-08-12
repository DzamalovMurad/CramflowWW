import type { BadgeSpec, BadgeVariant } from '../badges';
import { IconLeaf } from './icons';

/**
 * Бейдж поверх фото товара. Геометрия и типографика — общие, вариант меняет
 * только цвет: иначе плашки начинают спорить друг с другом за внимание.
 * Классы .badge-* живут в styles/index.css рядом с остальными токенами.
 */
export default function Badge({ variant, label }: BadgeSpec) {
  return (
    <span className={`badge badge-${variant}`}>
      {/* Плашка скошена, содержимое возвращается в вертикаль: иначе буквы
          едут вместе с ней и читаются как случайный курсив. */}
      <span className="badge-in">
        {variant === 'season' && <IconLeaf size={11} />}
        {label}
      </span>
    </span>
  );
}

/**
 * Стопка бейджей в левом верхнем углу фото — там, где на стоках пустой фон,
 * а не бутоны. pointer-events-none: плашка не должна перехватывать тап
 * по карточке.
 */
export function BadgeStack({ badges }: { badges: BadgeSpec[] }) {
  if (badges.length === 0) return null;
  return (
    <div className="pointer-events-none absolute left-2.5 top-2.5 flex flex-col items-start gap-1.5">
      {badges.map((b) => (
        <Badge key={b.variant} {...b} />
      ))}
    </div>
  );
}

export type { BadgeVariant };
