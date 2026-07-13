import { filterLabels } from '../content';
import { haptic } from '../telegram';

interface Props {
  selected: string;
  onSelect: (filter: string) => void;
}

/** Быстрые фильтры: серые плитки, активный — с лаймовой точкой. Комбинируются с категорией. */
export default function FilterPills({ selected, onSelect }: Props) {
  return (
    <div className="no-scrollbar flex gap-2 overflow-x-auto px-4 pb-3">
      {filterLabels.map(({ id, label }) => {
        const active = selected === id;
        return (
          <button
            key={id}
            onClick={() => {
              haptic('light');
              onSelect(active ? '' : id);
            }}
            className={`flex items-center gap-1.5 whitespace-nowrap rounded-button px-3.5 py-2 text-[13px] font-medium lowercase transition-colors ${
              active ? 'bg-tile text-ink' : 'bg-tile/60 text-muted'
            }`}
          >
            {active && <span className="h-1.5 w-1.5 rounded-full bg-accent-2" />}
            {label}
          </button>
        );
      })}
    </div>
  );
}
