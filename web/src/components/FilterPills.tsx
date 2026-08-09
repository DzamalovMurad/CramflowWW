import { content, filterLabels } from '../content';
import { haptic } from '../telegram';
import { IconLeaf } from './icons';

interface Props {
  selected: string;
  onSelect: (filter: string) => void;
  /** Сезонная таблетка «к 1 сентября» — необязательный акцент кампании. */
  seasonal?: { active: boolean; onToggle: (next: boolean) => void };
}

/** Быстрые фильтры (стиль Bunch): капсом-таблетки, активный — неоновая заливка. */
export default function FilterPills({ selected, onSelect, seasonal }: Props) {
  const pills = [{ id: '', label: 'все цветы' }, ...filterLabels];
  const pillBase =
    'min-h-[44px] whitespace-nowrap rounded-button border px-3.5 text-[11px] font-semibold uppercase tracking-[0.08em] transition-colors';
  return (
    <div className="no-scrollbar flex gap-2 overflow-x-auto px-4 pb-3">
      {seasonal && (
        <button
          onClick={() => {
            haptic('light');
            seasonal.onToggle(!seasonal.active);
          }}
          aria-pressed={seasonal.active}
          className={`${pillBase} flex items-center gap-1.5 ${
            seasonal.active ? 'pill-season pill-season-active' : 'pill-season'
          }`}
        >
          <IconLeaf size={12} />
          {content.seasonal.pill}
        </button>
      )}
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
