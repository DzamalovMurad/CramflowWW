import { useEffect, useMemo, useRef, useState } from 'react';
import { fetchMetroStations } from '../api';
import { findStation, searchStations } from '../metro';
import { content } from '../content';
import { haptic } from '../telegram';
import { IconCheck, IconClose, IconSearch } from './icons';

const c = content.delivery;

/**
 * Выбор станции московского метро: поиск с опечатками по всему списку станций.
 * Наверх поднимается только название из списка — произвольный текст сервер
 * не примет (проверка в service.CreateOrder).
 */
export default function MetroPicker({
  value,
  onChange,
}: {
  value: string;
  onChange: (station: string) => void;
}) {
  const [stations, setStations] = useState<string[]>([]);
  const [query, setQuery] = useState('');
  const [focused, setFocused] = useState(false);
  const blurTimer = useRef<number>();

  useEffect(() => {
    fetchMetroStations().then(setStations).catch(() => {});
    return () => window.clearTimeout(blurTimer.current);
  }, []);

  const matches = useMemo(() => searchStations(stations, query), [stations, query]);

  const pick = (station: string) => {
    haptic('light');
    onChange(station);
    setQuery('');
    setFocused(false);
  };

  // Выбранная станция показывается плашкой вместо поля — так видно,
  // что выбор сделан, и его нельзя случайно оставить недоделанным.
  if (value) {
    return (
      <div className="flex items-center justify-between rounded-input border border-accent/50 bg-surface px-4 py-3.5 shadow-[0_0_10px_rgba(128,255,0,0.12)]">
        <span className="flex min-w-0 items-center gap-2.5">
          <span className="text-accent-2">
            <IconCheck size={16} />
          </span>
          <span className="truncate text-[15px] font-medium">{value}</span>
        </span>
        <button
          type="button"
          onClick={() => {
            haptic('light');
            onChange('');
          }}
          className="ml-3 flex-shrink-0 text-sm lowercase text-muted active:opacity-50"
        >
          {c.metroChange}
        </button>
      </div>
    );
  }

  return (
    <div className="relative">
      <div className="flex items-center gap-2 rounded-input border border-line bg-surface px-4 py-3.5 transition-all duration-200 focus-within:border-accent focus-within:shadow-[0_0_10px_rgba(128,255,0,0.2)]">
        <span className="flex-shrink-0 text-muted">
          <IconSearch size={17} />
        </span>
        <input
          className="min-w-0 flex-1 bg-transparent text-[15px] text-ink outline-none placeholder:text-muted"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onFocus={() => setFocused(true)}
          // Клик по варианту сначала снимает фокус — даём ему сработать.
          onBlur={() => {
            blurTimer.current = window.setTimeout(() => setFocused(false), 150);
          }}
          onKeyDown={(e) => {
            if (e.key !== 'Enter') return;
            e.preventDefault(); // Enter в поле поиска не отправляет форму
            const exact = findStation(stations, query);
            if (exact) pick(exact);
            else if (matches.length === 1) pick(matches[0]);
          }}
          placeholder={c.metroPlaceholder}
          autoComplete="off"
        />
        {query && (
          <button
            type="button"
            aria-label="очистить"
            onClick={() => setQuery('')}
            className="flex-shrink-0 text-muted active:opacity-50"
          >
            <IconClose size={15} />
          </button>
        )}
      </div>

      {focused && (
        <div className="animate-fade-in absolute z-30 mt-1.5 max-h-64 w-full overflow-y-auto rounded-input border border-line bg-surface shadow-card">
          {matches.length === 0 ? (
            <p className="px-4 py-3 text-sm lowercase text-muted">{c.metroNotFound}</p>
          ) : (
            matches.map((station) => (
              <button
                type="button"
                key={station}
                onClick={() => pick(station)}
                className="block w-full border-b border-line px-4 py-3 text-left text-[15px] last:border-b-0 active:bg-tile"
              >
                {station}
              </button>
            ))
          )}
        </div>
      )}
      <p className="mt-1.5 text-xs lowercase text-muted">{c.metroHint}</p>
    </div>
  );
}
