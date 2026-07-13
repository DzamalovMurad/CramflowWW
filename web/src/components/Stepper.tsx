import { haptic } from '../telegram';
import { IconMinus, IconPlus } from './icons';

interface Props {
  value: number;
  onChange: (value: number) => void;
  min?: number;
}

/** Счётчик количества: серая плитка с − n +. */
export default function Stepper({ value, onChange, min = 1 }: Props) {
  const btn = 'flex h-9 w-9 items-center justify-center text-ink active:opacity-50';
  return (
    <div className="flex items-center rounded-button bg-tile">
      <button
        aria-label="уменьшить"
        className={btn}
        onClick={() => {
          haptic('light');
          onChange(Math.max(min - 1, value - 1));
        }}
      >
        <IconMinus size={15} />
      </button>
      <span className="min-w-7 text-center text-sm font-bold">{value}</span>
      <button
        aria-label="увеличить"
        className={btn}
        onClick={() => {
          haptic('light');
          onChange(Math.min(99, value + 1));
        }}
      >
        <IconPlus size={15} />
      </button>
    </div>
  );
}
