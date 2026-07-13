import { CATEGORIES } from '../types';
import { categoryLabels } from '../content';
import { haptic } from '../telegram';

interface Props {
  selected: string;
  onSelect: (category: string) => void;
}

/** Чипы категорий: строчные, монохром; активная — инверсия. Повторный тап снимает фильтр. */
export default function CategoryChips({ selected, onSelect }: Props) {
  return (
    <div className="no-scrollbar flex snap-x snap-mandatory gap-2 overflow-x-auto px-4 py-3">
      {CATEGORIES.map(({ name }) => {
        const active = selected === name;
        return (
          <button
            key={name}
            onClick={() => {
              haptic('light');
              onSelect(active ? '' : name);
            }}
            className={`snap-start whitespace-nowrap rounded-button border px-4 py-2 text-sm font-medium lowercase transition-colors ${
              active ? 'border-ink bg-ink text-page' : 'border-line bg-page text-ink'
            }`}
          >
            {categoryLabels[name] ?? name}
          </button>
        );
      })}
    </div>
  );
}
