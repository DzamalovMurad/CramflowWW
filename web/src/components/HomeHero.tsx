import { Link } from 'react-router-dom';
import { content } from '../content';
import { haptic } from '../telegram';

const c = content.home;

/**
 * Первый экран: заявление вместо приветствия.
 *
 * Блок не карточка на светлом фоне, а продолжение тёмной шапки — логотип,
 * поиск и заявление образуют один чёрный массив, который заканчивается
 * скруглением. Отдельная карточка дала бы третью горизонтальную полосу
 * подряд: ровно тот разрыв верха, от которого мы уходили.
 *
 * Цвета фиксированы вне темы, как у шапки и марки: первый экран обязан
 * выглядеть одинаково днём и ночью.
 */
export default function HomeHero() {
  return (
    <section className="hero">
      <p className="hero-label">{c.badge}</p>
      <h1 className="hero-title">{c.title}</h1>
      <p className="hero-sub">{c.subtitle}</p>
      <Link to="/catalog" onClick={() => haptic('light')} className="hero-cta cta">
        {c.heroCta}
      </Link>
    </section>
  );
}
