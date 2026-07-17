import { CATEGORIES } from '../types';
import { categoryLabels } from '../content';
import { haptic } from '../telegram';

interface Props {
  selected: string;
  onSelect: (category: string) => void;
}

/** Вкладки категорий (стиль Bunch): «Все» + категории. Активная — чёрная таблетка. */
export default function CategoryChips({ selected, onSelect }: Props) {
  const tabs = [{ name: '', label: 'все' }, ...CATEGORIES.map((c) => ({ name: c.name, label: categoryLabels[c.name] ?? c.name }))];
  return (
    <div className="no-scrollbar flex snap-x snap-mandatory items-center gap-1.5 overflow-x-auto px-4 py-3">
      {tabs.map(({ name, label }) => {
        const active = selected === name;
        return (
          <button
            key={name || 'all'}
            onClick={() => {
              haptic('light');
              onSelect(name);
            }}
            className={`tab snap-start rounded-button px-4 py-2 text-[15px] lowercase ${
              active ? 'bg-ink !text-page' : ''
            }`}
          >
            {label}
          </button>
        );
      })}
    </div>
  );
}
