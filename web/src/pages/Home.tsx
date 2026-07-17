import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import Header from '../components/Header';
import { content, categoryLabels } from '../content';
import { CATEGORIES } from '../types';
import { fetchFreshToday } from '../api';

const c = content.home;

/** Главная: иммерсивный герой с фрост-стеклом, блок «сегодня на базе», плитки категорий. */
export default function Home() {
  const imgRef = useRef<HTMLImageElement>(null);
  const [freshToday, setFreshToday] = useState<string | null>(null);

  // Лёгкий parallax героя — только transform, 60fps.
  useEffect(() => {
    let raf = 0;
    const onScroll = () => {
      cancelAnimationFrame(raf);
      raf = requestAnimationFrame(() => {
        if (imgRef.current) {
          imgRef.current.style.transform = `translateY(${Math.min(window.scrollY, 400) * 0.3}px)`;
        }
      });
    };
    window.addEventListener('scroll', onScroll, { passive: true });
    return () => {
      window.removeEventListener('scroll', onScroll);
      cancelAnimationFrame(raf);
    };
  }, []);

  useEffect(() => {
    fetchFreshToday()
      .then((data) => data.items && setFreshToday(data.items))
      .catch(() => {});
  }, []);

  return (
    <div className="pb-24">
      <Header />

      {/* ГЕРОЙ */}
      <section className="relative h-[80vh] min-h-[500px] overflow-hidden">
        <img
          ref={imgRef}
          src="/seed/hero.webp"
          alt=""
          className="absolute inset-0 h-[120%] w-full object-cover will-change-transform"
        />
        <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/25 to-black/15" />

        {/* Плашка-чип сверху */}
        <div className="absolute inset-x-0 top-0 flex justify-center pt-4">
          <span className="animate-fade-up flex items-center gap-1.5 rounded-full border border-white/25 bg-white/10 px-4 py-2 text-[12px] font-semibold lowercase text-white backdrop-blur-md">
            <span className="h-1.5 w-1.5 rounded-full bg-accent" />
            {c.badge}
          </span>
        </div>

        {/* Фрост-карточка с заголовком и CTA */}
        <div className="absolute inset-x-0 bottom-0 p-4">
          <div
            className="animate-fade-up rounded-[26px] border border-white/20 bg-white/10 p-5 shadow-float backdrop-blur-2xl"
            style={{ animationDelay: '100ms' }}
          >
            <h1 className="display text-[32px] leading-[1.02] text-white">{c.title}</h1>
            <p className="mt-2.5 text-[13px] lowercase leading-relaxed text-white/85">{c.subtitle}</p>
            <Link
              to="/catalog"
              className="btn-accent mt-4 flex w-full items-center justify-center rounded-button py-4 text-[15px] font-bold lowercase"
            >
              {c.cta}
            </Link>
          </div>
        </div>
      </section>

      {/* СЕГОДНЯ НА БАЗЕ */}
      {freshToday && (
        <section className="px-4 pt-5">
          <div className="animate-fade-up rounded-card border border-line bg-surface p-4 shadow-card">
            <p className="label mb-1.5 flex items-center gap-2">
              <span className="relative flex h-2 w-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-accent opacity-70" />
                <span className="relative inline-flex h-2 w-2 rounded-full bg-accent" />
              </span>
              {c.freshToday}
            </p>
            <p className="text-sm leading-relaxed text-ink">{freshToday}</p>
          </div>
        </section>
      )}

      {/* ПЛИТКИ КАТЕГОРИЙ */}
      <section className="px-4 pt-6">
        <h2 className="display mb-3 text-[22px]">{c.categories}</h2>
        <div className="grid grid-cols-2 gap-3">
          {CATEGORIES.map((cat, i) => (
            <Link
              key={cat.name}
              to={`/catalog?category=${encodeURIComponent(cat.name)}`}
              className="group animate-fade-up relative aspect-[4/3] overflow-hidden rounded-card shadow-card transition-transform active:scale-[0.97]"
              style={{ animationDelay: `${140 + i * 60}ms` }}
            >
              <img
                src={cat.image}
                alt=""
                loading="lazy"
                className="absolute inset-0 h-full w-full object-cover transition-transform duration-500 group-active:scale-110"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-black/75 via-black/10 to-transparent" />
              <div className="absolute inset-x-0 bottom-0 flex items-center justify-between p-3">
                <span className="text-[15px] font-bold lowercase text-white">
                  {categoryLabels[cat.name] ?? cat.name}
                </span>
                <span className="flex h-7 w-7 items-center justify-center rounded-full bg-accent text-[15px] font-bold text-on-accent">
                  →
                </span>
              </div>
            </Link>
          ))}
        </div>
      </section>
    </div>
  );
}
