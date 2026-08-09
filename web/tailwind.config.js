/**
 * Дизайн-токены CramFlow подключены через CSS-переменные —
 * править цвета/шрифты/радиусы нужно в src/styles/index.css (секция токенов),
 * тексты — в src/content.ts. Тёмная тема переключается атрибутом data-theme.
 */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        page: 'var(--c-bg)',
        surface: 'var(--c-surface)',
        tile: 'var(--c-tile)',
        ink: 'var(--c-text)',
        muted: 'var(--c-muted)',
        line: 'var(--c-border)',
        accent: 'var(--c-accent)',
        'accent-2': 'var(--c-accent-2)',
        'accent-ink': 'var(--c-accent-on-ink)', // акцент поверх bg-ink
        'on-accent': 'var(--c-on-accent)',
      },
      borderRadius: {
        card: 'var(--radius-card)',
        input: 'var(--radius-input)',
        button: 'var(--radius-button)',
      },
      boxShadow: {
        card: 'var(--shadow-card)',
        float: 'var(--shadow-float)',
      },
      fontFamily: {
        // Manrope — весь интерфейс; display (Cormorant Garamond) — только
        // брендовые заголовки, поэтому отдельной утилитой, а не по умолчанию.
        sans: 'var(--font-body)',
        display: 'var(--font-display)',
      },
    },
  },
  plugins: [],
};
