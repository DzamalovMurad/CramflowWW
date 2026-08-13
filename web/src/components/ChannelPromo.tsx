import { CHANNEL_URL, content } from '../content';
import { haptic, openTelegramLink } from '../telegram';

const c = content.home;

/**
 * Переход в канал магазина.
 *
 * Не кнопка «перейти» и не просьба подписаться: причина зайти лежит в самой
 * фразе — свежую поставку разбирают за день, и кто увидел её первым, тот её
 * и забрал. Плашка кликабельна целиком.
 *
 * Чартрез на весь блок: hero над ним чёрный, и второй тёмный блок подряд
 * слился бы с ним в одну массу. Цвета фиксированы вне темы, как у hero
 * и бейджей, — плашка обязана выглядеть одинаково днём и ночью.
 */
export default function ChannelPromo() {
  return (
    <button
      type="button"
      onClick={() => {
        haptic('light');
        openTelegramLink(CHANNEL_URL);
      }}
      className="promo"
    >
      <span className="promo-tag">{c.channelTag}</span>
      <span className="promo-title">{c.channelTitle}</span>
      <span className="promo-note">{c.channelNote}</span>
      {/* Стрелка вместо слова «перейти»: направление понятно без подписи. */}
      <span aria-hidden className="promo-arrow">
        →
      </span>
    </button>
  );
}
