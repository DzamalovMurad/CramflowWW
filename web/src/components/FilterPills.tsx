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
    'whitespace-nowrap rounded-button border px-3.5 py-2 font-mono text-[12px] font-bold uppercase tracking-wide transition-colors';
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
            onClick={() => {
              haptic('light');
              onSelect(id);
            }}
            className={`${pillBase} ${
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
