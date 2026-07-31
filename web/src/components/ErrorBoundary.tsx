import { Component, type ErrorInfo, type ReactNode } from 'react';
import { content } from '../content';
import { openBotChat } from '../telegram';

interface Props {
  children: ReactNode;
}

interface State {
  failed: boolean;
}

/**
 * Последний рубеж Mini App: любая необработанная ошибка рендера показывает
 * экран «что-то пошло не так» с выходом в чат бота — там заказ можно оформить
 * диалогом (/order). Белый экран внутри Telegram выглядит как сломанный
 * магазин и обрывает продажу, поэтому его быть не должно.
 */
export default class ErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Консоль — единственный доступный канал: телеметрию клиента не собираем.
    console.error('TMA crash:', error, info.componentStack);
  }

  render() {
    if (!this.state.failed) return this.props.children;

    const c = content.crash;
    return (
      <div className="flex min-h-screen flex-col items-center justify-center px-10 text-center">
        <div className="flex h-16 w-16 items-center justify-center rounded-full bg-tile text-[26px]">🥀</div>
        <p className="display mt-5 text-[22px]">{c.title}</p>
        <p className="mt-2 text-sm lowercase leading-relaxed text-muted">{c.hint}</p>

        <button
          type="button"
          onClick={() => openBotChat()}
          className="btn-accent mt-7 w-full max-w-[280px] rounded-button py-3.5 text-[15px] font-bold lowercase text-on-accent"
        >
          {c.openBot}
        </button>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="mt-3 text-sm lowercase text-muted underline underline-offset-4"
        >
          {c.retry}
        </button>
      </div>
    );
  }
}
