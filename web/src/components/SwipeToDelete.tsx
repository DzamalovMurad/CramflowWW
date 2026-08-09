import { useRef, useState, type ReactNode } from 'react';
import { haptic } from '../telegram';
import { IconTrash } from './icons';

const OPEN_X = -84; // на сколько уезжает карточка, открывая кнопку удаления

/**
 * Swipe-to-delete: контент смахивается влево, под ним — красная кнопка с корзиной.
 * Pointer Events (палец и мышь), без библиотек; вертикальный скролл не блокируется
 * (touch-action: pan-y), горизонтальный жест перехватываем сами.
 */
export default function SwipeToDelete({
  onDelete,
  children,
}: {
  onDelete: () => void;
  children: ReactNode;
}) {
  const [dx, setDx] = useState(0);
  const [dragging, setDragging] = useState(false);
  const start = useRef<{ x: number; y: number; dx: number; horizontal: boolean | null } | null>(null);
  const justDragged = useRef(false); // click сразу после жеста — не закрытие

  const onPointerDown = (e: React.PointerEvent) => {
    start.current = { x: e.clientX, y: e.clientY, dx, horizontal: null };
  };

  const onPointerMove = (e: React.PointerEvent) => {
    const s = start.current;
    if (!s) return;
    const moveX = e.clientX - s.x;
    const moveY = e.clientY - s.y;
    // Определяем намерение один раз: горизонтальный жест — наш, вертикальный — скроллу.
    if (s.horizontal === null) {
      if (Math.abs(moveX) < 6 && Math.abs(moveY) < 6) return;
      s.horizontal = Math.abs(moveX) > Math.abs(moveY);
      if (s.horizontal) {
        setDragging(true);
        (e.target as Element).setPointerCapture?.(e.pointerId);
      }
    }
    if (!s.horizontal) return;
    setDx(Math.max(OPEN_X - 20, Math.min(0, s.dx + moveX)));
  };

  const settle = () => {
    const s = start.current;
    start.current = null;
    setDragging(false);
    if (!s || s.horizontal !== true) return;
    justDragged.current = true;
    setDx((cur) => {
      const open = cur < OPEN_X / 2;
      if (open) haptic('light');
      return open ? OPEN_X : 0;
    });
  };

  return (
    <div className="relative overflow-hidden">
      {/* Нижний слой: красная кнопка удаления */}
      <button
        type="button"
        aria-label="удалить из корзины"
        onClick={() => {
          haptic('medium');
          onDelete();
        }}
        className="absolute inset-y-0 right-0 flex w-[84px] items-center justify-center bg-red-500 text-white"
      >
        <IconTrash size={22} />
      </button>

      {/* Верхний слой: контент, уезжает влево */}
      <div
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={settle}
        onPointerCancel={settle}
        style={{
          transform: `translateX(${dx}px)`,
          transition: dragging ? 'none' : 'transform 0.25s cubic-bezier(0.22, 1, 0.36, 1)',
          touchAction: 'pan-y',
        }}
        className="relative bg-page"
        onClickCapture={(e) => {
          // Клик, порождённый самим жестом, глушим без закрытия.
          if (justDragged.current) {
            justDragged.current = false;
            e.preventDefault();
            e.stopPropagation();
            return;
          }
          // Открытая карточка: обычный тап закрывает, не проваливаясь в ссылки.
          if (dx !== 0) {
            e.preventDefault();
            e.stopPropagation();
            setDx(0);
          }
        }}
      >
        {children}
      </div>
    </div>
  );
}
