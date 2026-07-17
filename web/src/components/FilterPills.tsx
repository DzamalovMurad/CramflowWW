import { filterLabels } from '../content';
import { haptic } from '../telegram';

interface Props {
  selected: string;
  onSelect: (filter: string) => void;
}

/** Быстрые фильтры (стиль Bunch): капсом-таблетки, активный — неоновая заливка. */
export default function FilterPills({ selected, onSelect }: Props) {
  const pills = [{ id: '', label: 'все цветы' }, ...filterLabels];
  return (
    <div className="no-scrollbar flex gap-2 overflow-x-auto px-4 pb-3">
      {pills.map(({ id, label }) => {
        const active = selected === id;
        return (
          <button
            key={id || 'all'}
            onClick={() => {
              haptic('light');
              onSelect(id);
            }}
            className={`whitespace-nowrap rounded-button border px-3.5 py-2 text-[12px] font-bold uppercase tracking-wide transition-colors ${
              active
                ? 'border-transparent bg-accent text-on-accent'
                : 'border-line bg-surface text-muted'
            }`}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
