import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import type { CartItem } from './types';

const STORAGE_KEY = 'flowix_cart_v1';
const MAX_QTY = 99;
const MAX_LINES = 20; // столько же принимает сервер

interface CartState {
  items: CartItem[];
  total: number;
  count: number;
  add: (item: Omit<CartItem, 'qty'>, qty?: number) => void;
  addMany: (items: Omit<CartItem, 'qty'>[], quantities: number[]) => void;
  setQty: (variantId: number, qty: number) => void;
  remove: (variantId: number) => void;
  /** Убрать разом позиции, которых больше нет в продаже. */
  removeMany: (variantIds: number[]) => void;
  clear: () => void;
}

const CartContext = createContext<CartState | null>(null);

/** Корзина переживает перезагрузку и сворачивание приложения. */
function load(): CartItem[] {
  try {
    const raw = JSON.parse(localStorage.getItem(STORAGE_KEY) || '[]');
    if (!Array.isArray(raw)) return [];
    // Чужие или испорченные данные не должны ронять приложение на старте.
    return raw.filter(
      (i: unknown): i is CartItem =>
        !!i &&
        typeof i === 'object' &&
        typeof (i as CartItem).variantId === 'number' &&
        typeof (i as CartItem).price === 'number' &&
        typeof (i as CartItem).qty === 'number' &&
        (i as CartItem).qty > 0,
    );
  } catch {
    return [];
  }
}

export function CartProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<CartItem[]>(load);

  useEffect(() => {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
    } catch {
      // Приватный режим или переполненное хранилище — корзина просто
      // не переживёт перезагрузку, но работать приложение не перестанет.
    }
  }, [items]);

  const add = useCallback((item: Omit<CartItem, 'qty'>, qty = 1) => {
    setItems((prev) => {
      const existing = prev.find((i) => i.variantId === item.variantId);
      if (existing) {
        return prev.map((i) =>
          i.variantId === item.variantId ? { ...i, qty: Math.min(MAX_QTY, i.qty + qty) } : i,
        );
      }
      if (prev.length >= MAX_LINES) return prev;
      return [...prev, { ...item, qty: Math.min(MAX_QTY, qty) }];
    });
  }, []);

  // addMany нужен для «повторить заказ»: одним обновлением состояния,
  // без гонки последовательных add().
  const addMany = useCallback((newItems: Omit<CartItem, 'qty'>[], quantities: number[]) => {
    setItems((prev) => {
      const next = [...prev];
      newItems.forEach((item, idx) => {
        const qty = Math.min(MAX_QTY, Math.max(1, quantities[idx] ?? 1));
        const existing = next.findIndex((i) => i.variantId === item.variantId);
        if (existing >= 0) {
          next[existing] = { ...next[existing], qty: Math.min(MAX_QTY, next[existing].qty + qty) };
        } else if (next.length < MAX_LINES) {
          next.push({ ...item, qty });
        }
      });
      return next;
    });
  }, []);

  const setQty = useCallback((variantId: number, qty: number) => {
    setItems((prev) =>
      qty <= 0
        ? prev.filter((i) => i.variantId !== variantId)
        : prev.map((i) =>
            i.variantId === variantId ? { ...i, qty: Math.min(MAX_QTY, qty) } : i,
          ),
    );
  }, []);

  const remove = useCallback((variantId: number) => {
    setItems((prev) => prev.filter((i) => i.variantId !== variantId));
  }, []);

  const removeMany = useCallback((variantIds: number[]) => {
    if (variantIds.length === 0) return;
    const drop = new Set(variantIds);
    setItems((prev) => prev.filter((i) => !drop.has(i.variantId)));
  }, []);

  const clear = useCallback(() => setItems([]), []);

  const value = useMemo<CartState>(() => {
    const total = items.reduce((sum, i) => sum + i.price * i.qty, 0);
    const count = items.reduce((sum, i) => sum + i.qty, 0);
    return { items, total, count, add, addMany, setQty, remove, removeMany, clear };
  }, [items, add, addMany, setQty, remove, removeMany, clear]);

  return <CartContext.Provider value={value}>{children}</CartContext.Provider>;
}

export function useCart(): CartState {
  const ctx = useContext(CartContext);
  if (!ctx) throw new Error('useCart вне CartProvider');
  return ctx;
}
