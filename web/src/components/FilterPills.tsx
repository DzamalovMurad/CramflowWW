import { FILTERS } from '../types';
import { haptic } from '../telegram';

interface Props {
  selected: string;
  onSelect: (filter: string) => void;
}

/** Быстрые фильтры-пилюли; комбинируются с категорией. Повторный тап снимает фильтр. */
export default function FilterPills({ selected, onSelect }: Props) {
  return (
    <div className="no-scrollbar flex gap-2 overflow-x-auto px-4 pb-3">
      {FILTERS.map(({ id, label }) => {
        const active = selected === id;
        return (
          <button
            key={id}
            onClick={() => {
              haptic('light');
              onSelect(active ? '' : id);
            }}
            className={`whitespace-nowrap rounded-full border px-3 py-1.5 text-xs font-medium transition-colors ${
              active ? 'border-accent-2 bg-accent-2/15 text-ink' : 'border-line text-muted'
            }`}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
