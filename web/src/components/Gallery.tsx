import { useRef, useState } from 'react';
import type { ProductImage } from '../types';
import { IconClose } from './icons';

/**
 * Свайп-галерея фото товара: scroll-snap, индикатор-точки,
 * фуллскрин по тапу (закрытие тапом/крестиком), lazy-load.
 */
export default function Gallery({ images, alt }: { images: ProductImage[]; alt: string }) {
  const [active, setActive] = useState(0);
  const [fullscreen, setFullscreen] = useState(false);
  const trackRef = useRef<HTMLDivElement>(null);

  const onScroll = () => {
    const el = trackRef.current;
    if (!el) return;
    setActive(Math.round(el.scrollLeft / el.clientWidth));
  };

  if (images.length === 0) {
    return <div className="aspect-square w-full bg-tile" />;
  }

  return (
    <>
      <div className="relative">
        <div
          ref={trackRef}
          onScroll={onScroll}
          className="no-scrollbar flex aspect-square w-full snap-x snap-mandatory overflow-x-auto bg-tile"
        >
          {images.map((img, i) => (
            <img
              key={img.id ?? i}
              src={img.url}
              alt={`${alt} — фото ${i + 1}`}
              loading={i === 0 ? 'eager' : 'lazy'}
              decoding="async"
              onClick={() => setFullscreen(true)}
              className="h-full w-full flex-shrink-0 snap-center object-cover"
            />
          ))}
        </div>
        {images.length > 1 && (
          <div className="absolute bottom-3 left-0 right-0 flex justify-center gap-1.5">
            {images.map((_, i) => (
              <span
                key={i}
                className={`h-1.5 rounded-full transition-all duration-300 ${
                  i === active ? 'w-5 bg-accent' : 'w-1.5 bg-white/70 shadow'
                }`}
              />
            ))}
          </div>
        )}
      </div>

      {fullscreen && (
        <div
          className="animate-fade-in fixed inset-0 z-50 flex items-center justify-center bg-black/95"
          onClick={() => setFullscreen(false)}
        >
          <button
            type="button"
            aria-label="закрыть"
            onClick={() => setFullscreen(false)}
            className="absolute right-4 z-10 flex h-11 w-11 items-center justify-center rounded-full bg-white/15 text-white"
            style={{ top: 'calc(env(safe-area-inset-top) + 16px)' }}
          >
            <IconClose size={18} />
          </button>
          <div className="no-scrollbar flex w-full snap-x snap-mandatory overflow-x-auto">
            {images.map((img, i) => (
              <img
                key={img.id ?? i}
                src={img.url}
                alt={`${alt} — фото ${i + 1}`}
                className="w-full flex-shrink-0 snap-center object-contain"
              />
            ))}
          </div>
        </div>
      )}
    </>
  );
}
