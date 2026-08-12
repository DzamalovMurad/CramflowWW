import { useEffect, useState, type FormEvent, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import { checkPromo, createOrder, fetchMe } from '../api';
import { useCart } from '../cart';
import { haptic, tg } from '../telegram';
import { content } from '../content';
import { DELIVERY_MODES, formatPrice, type DeliveryMode, type DeliveryType } from '../types';
import { IconCheck } from '../components/icons';
import MetroPicker from '../components/MetroPicker';

const c = content.checkout;
const d = content.delivery;

// Поля: плотный фон поверхности, тонкая рамка, неоновая подсветка при фокусе.
const inputCls =
  'w-full rounded-input border border-line bg-surface px-4 py-3.5 text-[15px] text-ink outline-none transition-all duration-200 placeholder:text-muted focus:border-accent focus:shadow-[0_0_10px_rgba(128,255,0,0.2)]';

function Field({ label, optional, children }: { label: string; optional?: string; children: ReactNode }) {
  return (
    <div>
      <label className="label mb-1.5 block">
        {label}
        {optional && <span className="text-muted"> · {optional}</span>}
      </label>
      {children}
    </div>
  );
}

/** Вариант доставки: крупная кликабельная строка с подписью-условием. */
function DeliveryOption({
  active,
  title,
  note,
  onSelect,
}: {
  active: boolean;
  title: string;
  note: string;
  onSelect: () => void;
}) {
  return (
    <button
      type="button"
      onClick={() => {
        haptic('light');
        onSelect();
      }}
      className={`flex items-center gap-3 rounded-input border px-4 py-3.5 text-left transition-all duration-200 active:scale-[0.99] ${
        active ? 'border-accent bg-surface shadow-[0_0_10px_rgba(128,255,0,0.15)]' : 'border-line bg-surface'
      }`}
    >
      <span
        aria-hidden
        className={`flex h-[22px] w-[22px] flex-shrink-0 items-center justify-center rounded-full border-2 transition-all duration-200 ${
          active ? 'neon-glow border-transparent bg-accent' : 'border-line'
        }`}
      >
        <span className={`text-on-accent transition-transform duration-200 ${active ? 'scale-100' : 'scale-0'}`}>
          <IconCheck size={12} />
        </span>
      </span>
      <span className="min-w-0">
        <span className="block text-[15px] lowercase text-ink">{title}</span>
        <span className="block text-xs lowercase text-muted">{note}</span>
      </span>
    </button>
  );
}

/** Оформление заказа: контакты, способ доставки, дата/время, комментарий, промокод. */
export default function Checkout() {
  const { items, total, clear } = useCart();
  const navigate = useNavigate();

  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  // Способ доставки выбирается обязательно: от него зависит, что спрашиваем дальше —
  // станцию метро (бесплатно) или адрес (курьер, цену называет менеджер).
  const [deliveryType, setDeliveryType] = useState<DeliveryType | ''>('');
  const [metroStation, setMetroStation] = useState('');
  const [address, setAddress] = useState('');
  const [date, setDate] = useState('');
  const [mode, setMode] = useState<DeliveryMode | ''>('');
  const [timeAt, setTimeAt] = useState(''); // HH:MM для режима «ко времени»
  const [comment, setComment] = useState('');
  const [cardText, setCardText] = useState('');
  const [isAnonymous, setIsAnonymous] = useState(false);

  const [promoInput, setPromoInput] = useState('');
  const [promo, setPromo] = useState<{ code: string; discount_percent: number } | null>(null);
  const [promoError, setPromoError] = useState('');
  const [promoSource, setPromoSource] = useState<'form' | 'deeplink' | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  const today = new Date().toISOString().slice(0, 10);

  // Подтягиваем имя/телефон прошлых заказов и промокод из deep-link бота.
  useEffect(() => {
    fetchMe()
      .then((me) => {
        if (me.name) setName((v) => v || me.name!);
        if (me.phone) setPhone((v) => v || me.phone!);
        if (me.promo_code && me.discount_percent) {
          setPromo({ code: me.promo_code, discount_percent: me.discount_percent });
          setPromoSource('deeplink');
        }
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    if (items.length === 0 && !submitting) navigate('/cart', { replace: true });
  }, [items.length, submitting, navigate]);

  const applyPromo = async () => {
    const code = promoInput.trim();
    if (!code) return;
    setPromoError('');
    try {
      const p = await checkPromo(code);
      setPromo(p);
      setPromoSource('form');
      haptic('success');
    } catch (e) {
      setPromo(null);
      setPromoSource(null);
      setPromoError((e as Error).message);
    }
  };

  const discounted = promo ? Math.floor((total * (100 - promo.discount_percent)) / 100) : total;

  // «в течение часа» уходит как есть, «ко времени» — как «к HH:MM».
  // Пустая строка — время не выбрано: это допустимо, поле необязательное.
  const deliveryTime = mode === 'ко времени' ? (timeAt ? `к ${timeAt}` : '') : mode;

  // Что обязательно заполнить, зависит от способа доставки.
  const deliveryReady =
    deliveryType === 'metro' ? !!metroStation : deliveryType === 'address' ? !!address.trim() : false;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      const order = await createOrder({
        items: items.map((i) => ({ variant_id: i.variantId, quantity: i.qty })),
        name,
        phone,
        delivery_type: deliveryType as DeliveryType,
        // Лишнее поле не отправляем — сервер его всё равно очистит.
        metro_station: deliveryType === 'metro' ? metroStation : '',
        delivery_address: deliveryType === 'address' ? address : '',
        delivery_date: date,
        delivery_time: deliveryTime,
        comment,
        card_text: cardText,
        is_anonymous: isAnonymous,
        // Промокод из deep-link сервер применит сам по initData.
        promo_code: promoSource === 'form' && promo ? promo.code : '',
      });
      clear();
      haptic('success');
      navigate(`/confirmation/${order.id}`, { state: { order }, replace: true });
    } catch (err) {
      setError((err as Error).message);
      setSubmitting(false);
    }
  };

  return (
    <div className="pb-10">
      <Header title={c.title} showBack={!tg()} />

      <form onSubmit={submit} className="space-y-5 p-4 pt-5">
        <Field label={c.name}>
          <input
            className={inputCls}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={c.namePlaceholder}
            required
          />
        </Field>

        <Field label={c.phone}>
          <input
            className={inputCls}
            type="tel"
            inputMode="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder={c.phonePlaceholder}
            required
          />
        </Field>

        <Field label={d.label}>
          <div className="grid gap-2">
            <DeliveryOption
              active={deliveryType === 'metro'}
              title={d.metroOption}
              note={d.metroOptionNote}
              onSelect={() => setDeliveryType('metro')}
            />
            <DeliveryOption
              active={deliveryType === 'address'}
              title={d.addressOption}
              note={d.addressOptionNote}
              onSelect={() => setDeliveryType('address')}
            />
          </div>
        </Field>

        {deliveryType === 'metro' && (
          <div className="animate-fade-in">
            <Field label={d.metroStation}>
              <MetroPicker value={metroStation} onChange={setMetroStation} />
            </Field>
          </div>
        )}

        {deliveryType === 'address' && (
          <div className="animate-fade-in">
            <Field label={c.address}>
              <input
                className={inputCls}
                value={address}
                onChange={(e) => setAddress(e.target.value)}
                placeholder={c.addressPlaceholder}
                required
              />
              {/* Курьера считает не приложение, а менеджер: предупреждаем до оплаты. */}
              <p className="mt-2 rounded-input border border-line bg-tile/60 px-3.5 py-3 text-xs leading-relaxed text-muted">
                {d.addressNote}
              </p>
            </Field>
          </div>
        )}

        <Field label={c.date}>
          <input
            className={inputCls}
            type="date"
            min={today}
            value={date}
            onChange={(e) => setDate(e.target.value)}
            required
          />
        </Field>

        {/* Время — пожелание клиента: до метро курьер подстроится,
            по адресу время всё равно согласует менеджер. */}
        <Field label={c.time} optional={c.timeOptional}>
          <div className="grid grid-cols-2 gap-2">
            {DELIVERY_MODES.map((m) => (
              <button
                type="button"
                key={m}
                onClick={() => {
                  haptic('light');
                  setMode((prev) => (prev === m ? '' : m)); // повторный тап снимает выбор
                }}
                className={`rounded-input border py-3.5 font-mono text-[12px] font-bold uppercase tracking-wide transition-all duration-200 active:scale-[0.97] ${
                  mode === m
                    ? 'neon-glow border-transparent bg-accent text-on-accent'
                    : 'border-line bg-surface text-muted'
                }`}
              >
                {m}
              </button>
            ))}
          </div>
          {mode === 'ко времени' && (
            <div className="animate-fade-in mt-2">
              <input
                className={inputCls}
                type="time"
                min="09:00"
                max="21:00"
                value={timeAt}
                onChange={(e) => setTimeAt(e.target.value)}
                required
              />
            </div>
          )}
          <p className="mt-1.5 text-xs lowercase text-muted">
            {deliveryType === 'address' ? d.timeManaged : c.timeNote}
          </p>
        </Field>

        <Field label={c.comment} optional={c.commentOptional}>
          <textarea
            className={`${inputCls} resize-none`}
            rows={2}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder={c.commentPlaceholder}
          />
        </Field>

        <Field label={c.cardText} optional={c.cardTextOptional}>
          <textarea
            className={`${inputCls} resize-none`}
            rows={2}
            value={cardText}
            onChange={(e) => setCardText(e.target.value)}
            placeholder={c.cardTextPlaceholder}
            maxLength={300}
          />
          {cardText && (
            <p className="mt-1 text-xs text-muted">{cardText.length} / 300</p>
          )}
        </Field>

        <div>
          <label className="flex cursor-pointer items-center gap-3 rounded-input border border-line bg-surface px-4 py-3.5 transition-colors">
            <input
              type="checkbox"
              checked={isAnonymous}
              onChange={(e) => {
                haptic('light');
                setIsAnonymous(e.target.checked);
              }}
              className="sr-only"
            />
            <span
              aria-hidden
              className={`flex h-[22px] w-[22px] flex-shrink-0 items-center justify-center rounded-md border-2 transition-all duration-200 ${
                isAnonymous ? 'neon-glow border-transparent bg-accent' : 'border-line bg-transparent'
              }`}
            >
              <span
                className={`text-on-accent transition-transform duration-200 ${
                  isAnonymous ? 'scale-100' : 'scale-0'
                }`}
              >
                <IconCheck size={13} />
              </span>
            </span>
            <span className="text-[15px] lowercase text-ink">{c.anonymous}</span>
          </label>
        </div>

        <Field label={c.promo}>
          {promo ? (
            <div className="flex items-center justify-between rounded-input border border-accent/50 bg-surface px-4 py-3.5 shadow-[0_0_10px_rgba(128,255,0,0.12)]">
              <span className="flex items-center gap-2 font-mono text-[13px] font-bold uppercase">
                <span className="h-1.5 w-1.5 rounded-full bg-accent" />
                {promo.code} <span className="text-accent-2">−{promo.discount_percent}%</span>
              </span>
              {promoSource === 'form' && (
                <button
                  type="button"
                  className="text-sm lowercase text-muted"
                  onClick={() => {
                    setPromo(null);
                    setPromoSource(null);
                    setPromoInput('');
                  }}
                >
                  {c.promoRemove}
                </button>
              )}
            </div>
          ) : (
            <div className="flex gap-2">
              <input
                className={inputCls}
                value={promoInput}
                onChange={(e) => setPromoInput(e.target.value.toUpperCase())}
                placeholder={c.promoPlaceholder}
              />
              <button
                type="button"
                onClick={applyPromo}
                className="whitespace-nowrap rounded-input border border-line bg-surface px-4 font-mono text-[12px] font-bold uppercase tracking-wide text-ink transition-transform active:scale-95"
              >
                {c.promoApply}
              </button>
            </div>
          )}
          {promoError && <p className="mt-1.5 text-xs lowercase text-red-500">{promoError}</p>}
        </Field>

        {/* Итог: подписи и промежуточные суммы — mono, финальная сумма — жирный гротеск */}
        <div className="rounded-card border border-line bg-surface p-5">
          {promo && (
            <div className="mb-3 space-y-1.5 border-b border-line pb-3">
              <div className="flex justify-between font-mono text-[13px] uppercase text-muted">
                <span>{c.subtotal}</span>
                <span>{formatPrice(total)}</span>
              </div>
              <div className="flex justify-between font-mono text-[13px] uppercase text-accent-2">
                <span>
                  {c.discount} {promo.discount_percent}%
                </span>
                <span>−{formatPrice(total - discounted)}</span>
              </div>
            </div>
          )}
          <div className="flex items-baseline justify-between">
            <span className="label">{c.total}</span>
            <span className="text-[28px] font-extrabold tracking-tight">{formatPrice(discounted)}</span>
          </div>
          {/* В сумме только букеты. Доставка в неё не входит ни в каком виде:
              до метро она бесплатна, курьера клиент оплачивает отдельно. */}
          {deliveryType && (
            <p className="mt-2 text-xs lowercase leading-relaxed text-muted">
              {deliveryType === 'metro' ? d.totalNoteMetro : d.totalNoteAddress}
            </p>
          )}
        </div>

        {error && <p className="text-center text-sm lowercase text-red-500">{error}</p>}

        <button
          type="submit"
          disabled={submitting || !deliveryReady}
          className="btn-accent btn-accent-strong w-full rounded-button py-4 text-[15px] font-bold lowercase text-on-accent disabled:opacity-50"
        >
          {submitting ? c.submitting : c.submit}
        </button>
      </form>
    </div>
  );
}
