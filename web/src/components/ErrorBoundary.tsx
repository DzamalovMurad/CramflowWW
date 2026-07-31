import { Component, type ErrorInfo, type ReactNode } from 'react';
import { content } from '../content';

/**
 * Граница ошибок: без неё любая ошибка рендера оставляет клиента
 * перед белым экраном без единого способа выбраться.
 *
 * Корзина живёт в localStorage и переживает перезагрузку, поэтому «обновить» —
 * действительно рабочий выход, а не потеря собранного заказа.
 */
export default class ErrorBoundary extends Component<
  { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Ошибку видно в консоли Telegram WebView — этого достаточно, чтобы
    // разобраться, и не требует отдельного сервиса аналитики.
    console.error('Ошибка интерфейса:', error, info.componentStack);
  }

  render() {
    if (!this.state.failed) return this.props.children;

    return (
      <div className="flex min-h-screen flex-col items-center justify-center px-8 text-center">
        <div className="text-5xl">🥀</div>
        <h1 className="display mt-5 text-[24px]">{content.error.title}</h1>
        <p className="mt-2 text-sm lowercase leading-relaxed text-muted">{content.error.hint}</p>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="btn-accent mt-7 min-h-[48px] w-full max-w-[260px] rounded-button px-7 text-[15px] font-bold lowercase text-on-accent"
        >
          {content.error.reload}
        </button>
        <button
          type="button"
          onClick={() => {
            window.location.href = '/';
          }}
          className="mt-3 min-h-[44px] px-4 text-sm lowercase text-muted"
        >
          {content.error.toHome}
        </button>
      </div>
    );
  }
}
