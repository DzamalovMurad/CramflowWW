/**
 * Дизайн-токены CramFlow (премиум-минимализм).
 * Цвета заведены через CSS-переменные, чтобы тёмная тема Telegram
 * переключалась без пересборки (см. src/styles/index.css и src/telegram.ts).
 */
export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        page: 'var(--c-bg)',          // фон: #FFFFFF / тёмный
        surface: 'var(--c-surface)',  // карточки
        ink: 'var(--c-text)',         // текст: #111111 / светлый
        muted: 'var(--c-muted)',      // вторичный текст
        line: 'var(--c-border)',      // бордеры: #EDEDED / тёмный
        accent: '#A7FC00',            // акцент
        'accent-2': '#87CC00',        // secondary-акцент
      },
      borderRadius: {
        card: '8px',
      },
      boxShadow: {
        card: '0 4px 8px rgba(0,0,0,0.1)',
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'Segoe UI', 'Roboto', 'sans-serif'],
      },
      spacing: {
        // база 16px, отступы кратны 8 — стандартная шкала Tailwind уже кратна 4/8
      },
    },
  },
  plugins: [],
};
