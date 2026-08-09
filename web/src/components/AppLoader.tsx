import { useEffect, useState } from 'react';
import { content } from '../content';

/**
 * Стартовая заставка: пульсирующий wordmark на тёмном фоне.
 * Пока она видна, App греет каталог и первые фото (warmUp) —
 * пользователь попадает сразу на готовую витрину без скелетонов.
 */
export default function AppLoader({ done }: { done: boolean }) {
  const [gone, setGone] = useState(false);

  useEffect(() => {
    if (!done) return;
    const t = setTimeout(() => setGone(true), 500); // дать доиграть fade-out
    return () => clearTimeout(t);
  }, [done]);

  if (gone) return null;

  return (
    <div className={`app-loader ${done ? 'app-loader-out' : ''}`} aria-hidden={done}>
      <div className="loader-mark">
        {content.brand}
        <span className="loader-dot" />
      </div>
      <div className="loader-bar">
        <span />
      </div>
    </div>
  );
}
