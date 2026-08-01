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
            type="button"
            aria-pressed={active}
            onClick={() => {
              haptic('light');
              onSelect(id);
            }}
            className={`min-h-[44px] whitespace-nowrap rounded-button border px-3.5 text-[11px] font-semibold uppercase tracking-[0.08em] transition-colors ${
              active
                ? 'neon-glow border-transparent bg-accent text-on-accent'
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
