import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import type { CartItem } from './types';

const STORAGE_KEY = 'cramflow_cart_v1';

interface CartState {
  items: CartItem[];
  total: number;
  count: number;
  add: (item: Omit<CartItem, 'qty'>, qty?: number) => void;
  setQty: (variantId: number, qty: number) => void;
  remove: (variantId: number) => void;
  /** Переключение размера S/M/L прямо в корзине (другой вариант того же товара). */
  changeVariant: (variantId: number, next: { variantId: number; flowersCount: number; price: number }) => void;
  /** «Повторить заказ»: заменяет содержимое корзины целиком. */
  fill: (items: CartItem[]) => void;
  clear: () => void;
}

const CartContext = createContext<CartState | null>(null);

function load(): CartItem[] {
  try {
    return JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
  } catch {
    return [];
  }
}

export function CartProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<CartItem[]>(load);

  useEffect(() => {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
  }, [items]);

  const add = useCallback((item: Omit<CartItem, 'qty'>, qty = 1) => {
    setItems((prev) => {
      const existing = prev.find((i) => i.variantId === item.variantId);
      if (existing) {
        return prev.map((i) =>
          i.variantId === item.variantId ? { ...i, qty: Math.min(99, i.qty + qty) } : i,
        );
      }
      return [...prev, { ...item, qty }];
    });
  }, []);

  const setQty = useCallback((variantId: number, qty: number) => {
    setItems((prev) =>
      qty <= 0
        ? prev.filter((i) => i.variantId !== variantId)
        : prev.map((i) => (i.variantId === variantId ? { ...i, qty: Math.min(99, qty) } : i)),
    );
  }, []);

  const remove = useCallback((variantId: number) => {
    setItems((prev) => prev.filter((i) => i.variantId !== variantId));
  }, []);

  const changeVariant = useCallback(
    (variantId: number, next: { variantId: number; flowersCount: number; price: number }) => {
      setItems((prev) => {
        const src = prev.find((i) => i.variantId === variantId);
        if (!src || variantId === next.variantId) return prev;
        const dup = prev.find((i) => i.variantId === next.variantId);
        if (dup) {
          // Целевой размер уже в корзине — объединяем количество.
          return prev
            .filter((i) => i.variantId !== variantId)
            .map((i) => (i.variantId === next.variantId ? { ...i, qty: Math.min(99, i.qty + src.qty) } : i));
        }
        return prev.map((i) => (i.variantId === variantId ? { ...i, ...next } : i));
      });
    },
    [],
  );

  const fill = useCallback((next: CartItem[]) => setItems(next), []);

  const clear = useCallback(() => setItems([]), []);

  const value = useMemo<CartState>(() => {
    const total = items.reduce((sum, i) => sum + i.price * i.qty, 0);
    const count = items.reduce((sum, i) => sum + i.qty, 0);
    return { items, total, count, add, setQty, remove, changeVariant, fill, clear };
  }, [items, add, setQty, remove, changeVariant, fill, clear]);

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export function useCart(): CartState {
  const ctx = useContext(CartContext);
  if (!ctx) throw new Error('useCart вне CartProvider');
  return ctx;
}
