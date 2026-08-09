import { haptic } from '../telegram';
import { IconMinus, IconPlus } from './icons';

interface Props {
  value: number;
  onChange: (value: number) => void;
  min?: number;
  max?: number;
}

/**
 * Счётчик количества. Кнопки — 44×44: меньше не попасть пальцем,
 * а промах здесь означает случайно удалённую позицию.
 */
export default function Stepper({ value, onChange, min = 1, max = 99 }: Props) {
  const btn =
    'flex h-11 w-11 items-center justify-center text-ink active:opacity-50 disabled:opacity-30';
  return (
    <div className="flex items-center rounded-button bg-tile">
      <button
        type="button"
        aria-label="уменьшить"
        className={btn}
        disabled={value <= min - 1}
        onClick={() => {
          haptic('light');
          onChange(Math.max(min - 1, value - 1));
        }}
      >
        <IconMinus size={16} />
      </button>
      <span className="min-w-7 text-center text-sm font-bold tabular-nums">{value}</span>
      <button
        type="button"
        aria-label="увеличить"
        className={btn}
        disabled={value >= max}
        onClick={() => {
          haptic('light');
          onChange(Math.min(max, value + 1));
        }}
      >
        <IconPlus size={16} />
      </button>
    </div>
  );
}
