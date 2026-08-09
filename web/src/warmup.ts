import { fetchAllProducts } from './api';

/**
 * Прогрев стартового экрана: тянем каталог и первые фото в кэш браузера,
 * пока видна заставка. Ошибки глушим намеренно — заставка не должна
 * задерживать вход из-за проблем с сетью, каталог покажет свою ошибку сам.
 */
export async function warmUp(): Promise<void> {
  try {
    const list = await fetchAllProducts();
    await Promise.all(
      list.slice(0, 4).map(
        (p) =>
          new Promise<void>((resolve) => {
            if (!p.image) return resolve();
            const img = new Image();
            img.onload = img.onerror = () => resolve();
            img.src = p.image;
          }),
      ),
    );
  } catch {
    // Вход не блокируем.
  }
}
