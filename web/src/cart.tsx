import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import type { CartItem } from './types';
import { checkCart, touchCart } from './api';

const STORAGE_KEY = 'cramflow_cart_v1';

interface CartState {
  items: CartItem[];
  total: number;
  count: number;
  add: (item: Omit<CartItem, 'qty'>, qty?: number) => void;
  setQty: (variantId: number, qty: number) => void;
  remove: (variantId: number) => void;
  clear: () => void;
  /** variantId позиций, которые больше нельзя заказать (сняты с наличия). */
  unavailable: number[];
  /** Сверить состав корзины с каталогом. Зовётся при открытии корзины и checkout. */
  revalidate: () => Promise<number[]>;
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
  const [unavailable, setUnavailable] = useState<number[]>([]);

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
    touchCart(); // сегмент рассылки «корзина без заказа»
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

  const clear = useCallback(() => {
    setItems([]);
    setUnavailable([]);
  }, []);

  const revalidate = useCallback(async () => {
    const ids = items.map((i) => i.variantId);
    if (ids.length === 0) {
      setUnavailable([]);
      return [];
    }
    try {
      const { unavailable: gone } = await checkCart(ids);
      setUnavailable(gone);
      return gone;
    } catch {
      // Сеть отвалилась — не блокируем корзину: состав всё равно проверяется
      // на сервере при оформлении, недоступный товар туда не пройдёт.
      setUnavailable([]);
      return [];
    }
  }, [items]);

  const value = useMemo<CartState>(() => {
    // Недоступные позиции не участвуют в сумме: клиент заплатит за то,
    // что реально уедет, а не за строку, которую его просят удалить.
    const total = items.reduce(
      (sum, i) => (unavailable.includes(i.variantId) ? sum : sum + i.price * i.qty),
      0,
    );
    const count = items.reduce((sum, i) => sum + i.qty, 0);
    return { items, total, count, add, setQty, remove, clear, unavailable, revalidate };
  }, [items, add, setQty, remove, clear, unavailable, revalidate]);

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export function useCart(): CartState {
  const ctx = useContext(CartContext);
  if (!ctx) throw new Error('useCart вне CartProvider');
  return ctx;
}
