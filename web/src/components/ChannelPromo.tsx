import { CHANNEL_URL, content } from '../content';
import { haptic, openTelegramLink } from '../telegram';

const c = content.home;

/**
 * Переход в канал магазина.
 *
 * Не кнопка «перейти» и не просьба подписаться: причина зайти лежит в самой
 * фразе — свежую поставку разбирают за день, и кто увидел её первым, тот её
 * и забрал. Кликабелен весь блок.
 *
 * Постерная вёрстка: чёрная база, кислотный клин по диагонали, капс двумя
 * цветами и мелкая мета-строка. Плоская заливка, которая была здесь раньше,
 * читалась как баннер; клин и разные цвета строк дают глубину без картинок
 * и анимаций. Цвета фиксированы вне темы, как у hero и бейджей.
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
      <span className="promo-wedge" aria-hidden />
      <span className="promo-body">
        <span className="promo-meta">{c.channelMeta}</span>
        <span className="promo-title">
          <span className="promo-line-1">{c.channelTitleTop}</span>
          <span className="promo-line-2">{c.channelTitleBottom}</span>
        </span>
        <span className="promo-note">{c.channelNote}</span>
      </span>
      {/* Стрелка вместо слова «перейти»: направление понятно без подписи. */}
      <span aria-hidden className="promo-arrow">
        ↗
      </span>
    </button>
  );
}
