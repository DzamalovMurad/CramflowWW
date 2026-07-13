import { haptic } from '../telegram';

interface Props {
  value: number;
  onChange: (value: number) => void;
  min?: number;
}

/** Счётчик количества «− n +». */
export default function Stepper({ value, onChange, min = 1 }: Props) {
  const btn =
    'flex h-8 w-8 items-center justify-center rounded-full border border-line text-lg leading-none active:bg-line';
  return (
    <div className="flex items-center gap-3">
      <button
        aria-label="Уменьшить"
        className={btn}
        onClick={() => {
          haptic('light');
          onChange(Math.max(min - 1, value - 1));
        }}
      >
        −
      </button>
      <span className="min-w-6 text-center text-base font-semibold">{value}</span>
      <button
        aria-label="Увеличить"
        className={btn}
        onClick={() => {
          haptic('light');
          onChange(Math.min(99, value + 1));
        }}
      >
        +
      </button>
    </div>
  );
}
