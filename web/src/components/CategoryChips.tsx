import { CATEGORIES } from '../types';
import { haptic } from '../telegram';

interface Props {
  selected: string;
  onSelect: (category: string) => void;
}

/** Горизонтальные чипы категорий со snap-скроллом. Повторный тап снимает фильтр. */
export default function CategoryChips({ selected, onSelect }: Props) {
  return (
    <div className="no-scrollbar flex snap-x snap-mandatory gap-2 overflow-x-auto px-4 py-2">
      {CATEGORIES.map(({ emoji, name }) => {
        const active = selected === name;
        return (
          <button
            key={name}
            onClick={() => {
              haptic('light');
              onSelect(active ? '' : name);
            }}
            className={`snap-start whitespace-nowrap rounded-full border px-4 py-2 text-sm font-medium transition-colors ${
              active
                ? 'border-accent bg-accent text-[#111111]'
                : 'border-line bg-surface text-ink'
            }`}
          >
            {emoji} {name}
          </button>
        );
      })}
    </div>
  );
}
