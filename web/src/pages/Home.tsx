import { useEffect, useRef } from 'react';
import { Link } from 'react-router-dom';
import Header from '../components/Header';
import { CATEGORIES } from '../types';

/** Главный экран: hero с фото букета, заголовок и CTA. Лёгкий parallax на transform. */
export default function Home() {
  const imgRef = useRef<HTMLImageElement>(null);

  useEffect(() => {
    let raf = 0;
    const onScroll = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        if (imgRef.current) {
          imgRef.current.style.transform = `translateY(${window.scrollY * 0.35}px)`;
        }
      });
    };
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => {
      window.removeEventListener('scroll', onScroll);
      cancelAnimationFrame(raf);
    };
  }, []);

  return (
    <div className="pb-8">
      <Header />

      <section className="relative h-[62vh] min-h-[380px] overflow-hidden">
        <img
          ref={imgRef}
          src="/seed/hero.webp"
          alt="Букет цветов"
          className="absolute inset-0 h-[120%] w-full object-cover will-change-transform"
        />
        <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-black/10 to-transparent" />
        <div className="absolute bottom-0 left-0 right-0 p-6 pb-8 text-white">
          <h1 className="animate-fade-up text-3xl font-bold leading-tight">
            Цветы, которые
            <br />
            хочется дарить
          </h1>
          <p className="animate-fade-up mt-2 text-sm text-white/85" style={{ animationDelay: '80ms' }}>
            Свежие букеты с доставкой сегодня
          </p>
          <Link
            to="/catalog"
            className="animate-fade-up mt-5 block w-full rounded-card bg-accent py-3.5 text-center text-base font-semibold text-[#111111] shadow-card transition-transform active:scale-[0.98]"
            style={{ animationDelay: '160ms' }}
          >
            Перейти в каталог
          </Link>
        </div>
      </section>

      <section className="px-4 pt-6">
        <h2 className="mb-3 text-lg font-semibold">Категории</h2>
        <div className="grid grid-cols-2 gap-3">
          {CATEGORIES.map(({ emoji, name }, i) => (
            <Link
              key={name}
              to={`/catalog?category=${encodeURIComponent(name)}`}
              className="animate-fade-up flex items-center gap-3 rounded-card border border-line bg-surface p-4 shadow-card transition-transform active:scale-[0.97]"
              style={{ animationDelay: `${i * 60}ms` }}
            >
              <span className="text-2xl">{emoji}</span>
              <span className="text-sm font-medium">{name}</span>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
