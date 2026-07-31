import { CATEGORIES } from '../types';
import { categoryLabels } from '../content';
import { haptic } from '../telegram';

interface Props {
  selected: string;
  onSelect: (category: string) => void;
}

/** Вкладки категорий: «все» + категории. Активная — чёрная таблетка. */
export default function CategoryChips({ selected, onSelect }: Props) {
  const tabs: { name: string; label: string }[] = [
    { name: '', label: 'все' },
    ...CATEGORIES.map((name) => ({ name, label: categoryLabels[name] ?? name })),
  ];
  return (
    <div className="no-scrollbar flex snap-x snap-mandatory items-center gap-1.5 overflow-x-auto px-4 py-2">
      {tabs.map(({ name, label }) => {
        const active = selected === name;
        return (
          <button
            key={name || 'all'}
            type="button"
            aria-pressed={active}
            onClick={() => {
              haptic('light');
              onSelect(name);
            }}
            className={`tab snap-start min-h-[44px] rounded-button px-4 text-[15px] lowercase ${
              active ? 'neon-glow bg-ink !text-page' : ''
            }`}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
