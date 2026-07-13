import { useEffect, useRef, useState } from 'react';
import { Link } from 'react-router-dom';
import Header from '../components/Header';
import { content, categoryLabels } from '../content';
import { CATEGORIES } from '../types';
import { fetchFreshToday } from '../api';

/** Главный экран: фулскрин-фото, строчный заголовок, CTA. */
export default function Home() {
  const imgRef = useRef<HTMLImageElement>(null);
  const [freshToday, setFreshToday] = useState<string | null>(null);

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

  useEffect(() => {
    fetchFreshToday()
      .then((data) => {
        if (data.items) setFreshToday(data.items);
      })
      .catch(() => {});
  }, []);

  return (
    <div className="pb-10">
      <Header />

      <section className="relative h-[68vh] min-h-[420px] overflow-hidden">
        <img
          ref={imgRef}
          src="/seed/hero.webp"
          alt=""
          className="absolute inset-0 h-[120%] w-full object-cover will-change-transform"
        />
        <div className="absolute inset-0 bg-gradient-to-t from-black/75 via-black/15 to-transparent" />
        <div className="absolute bottom-0 left-0 right-0 p-5 pb-7 text-white">
          <h1 className="display animate-fade-up text-[34px]">{content.home.title}</h1>
          <p
            className="animate-fade-up mt-2.5 text-sm lowercase text-white/80"
            style={{ animationDelay: '80ms' }}
          >
            {content.home.subtitle}
          </p>
          <Link
            to="/catalog"
            className="btn-warm animate-fade-up mt-6 block w-full rounded-button py-4 text-center text-[15px] font-bold lowercase text-on-accent"
            style={{ animationDelay: '160ms' }}
          >
            {content.home.cta}
          </Link>
        </div>
      </section>

      {freshToday && (
        <section className="animate-fade-up border-b border-line px-5 py-6">
          <p className="label mb-2 block">сегодня на базе</p>
          <p className="text-sm leading-relaxed text-ink">{freshToday}</p>
        </section>
      )}

      <nav className="divide-y divide-line border-b border-line">
        {CATEGORIES.map(({ name }, i) => (
          <Link
            key={name}
            to={`/catalog?category=${encodeURIComponent(name)}`}
            className="animate-fade-up flex items-center justify-between px-5 py-4 active:bg-tile"
            style={{ animationDelay: `${200 + i * 50}ms` }}
          >
            <span className="text-[17px] font-bold lowercase tracking-tight">
              {categoryLabels[name] ?? name}
            </span>
            <span className="text-muted">→</span>
          </Link>
        ))}
      </nav>
    </div>
  );
}
