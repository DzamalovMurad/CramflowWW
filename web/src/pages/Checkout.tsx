import { useEffect, useMemo, useState, type FormEvent, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import { checkPromo, createOrder, fetchMe, fetchSlots, uuid } from '../api';
import { useCart } from '../cart';
import { clearPendingPromo, haptic, pendingPromo, tg } from '../telegram';
import { content } from '../content';
import { formatPrice, type PromoInfo, type SlotDay } from '../types';
import { IconCheck } from '../components/icons';

const c = content.checkout;

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

/** Чекбокс в стиле бренда (используется для «не себе», адреса и анонимности). */
function Toggle({ checked, onChange, label }: { checked: boolean; onChange: (v: boolean) => void; label: string }) {
  return (
    <label className="flex cursor-pointer items-center gap-3 rounded-input border border-line bg-surface px-4 py-3.5 transition-colors">
      <input
        type="checkbox"
        checked={checked}
        onChange={(e) => {
          haptic('light');
          onChange(e.target.checked);
        }}
        className="sr-only"
      />
      <span
        aria-hidden
        className={`flex h-[22px] w-[22px] flex-shrink-0 items-center justify-center rounded-md border-2 transition-all duration-200 ${
          checked ? 'neon-glow border-transparent bg-accent' : 'border-line bg-transparent'
        }`}
      >
        <span className={`text-on-accent transition-transform duration-200 ${checked ? 'scale-100' : 'scale-0'}`}>
          <IconCheck size={13} />
        </span>
      </span>
      <span className="text-[15px] lowercase text-ink">{label}</span>
    </label>
  );
}

/** Оформление заказа: контакты, получатель, дата и слот, открытка, промокод. */
export default function Checkout() {
  const { items, total, clear } = useCart();
  const navigate = useNavigate();

  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  const [address, setAddress] = useState('');

  // Слоты доставки: чипы дат и 2-часовых окон приходят с сервера.
  const [days, setDays] = useState<SlotDay[] | null>(null);
  const [date, setDate] = useState('');
  const [slot, setSlot] = useState('');

  // Подарочный флоу «заказываю не себе».
  const [notSelf, setNotSelf] = useState(false);
  const [recipientName, setRecipientName] = useState('');
  const [recipientPhone, setRecipientPhone] = useState('');
  const [addressByRecipient, setAddressByRecipient] = useState(false);

  const [comment, setComment] = useState('');
  const [cardText, setCardText] = useState('');
  const [isAnonymous, setIsAnonymous] = useState(false);

  const [promoInput, setPromoInput] = useState('');
  const [promo, setPromo] = useState<PromoInfo | null>(null);
  const [promoError, setPromoError] = useState('');
  const [promoSource, setPromoSource] = useState<'form' | 'deeplink' | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  // Ключ идемпотентности живёт всё время на экране: ретрай после таймаута
  // или двойной тап вернут тот же заказ, а не создадут дубль.
  const idempotencyKey = useMemo(uuid, []);

  const orderItems = useMemo(
    () => items.map((i) => ({ variant_id: i.variantId, quantity: i.qty })),
    [items],
  );

  useEffect(() => {
    fetchSlots()
      .then(({ days }) => {
        setDays(days);
        if (days.length > 0) setDate((d) => d || days[0].date);
      })
      .catch(() => setDays([]));
  }, []);

  // Подтягиваем имя/телефон прошлых заказов и авто-применяем промокод из
  // deep-link: startapp=promo_<CODE> приоритетнее сохранённого ботом кода.
  useEffect(() => {
    if (items.length === 0) return;
    const applyDeeplink = (code: string) =>
      checkPromo(code, orderItems)
        .then((p) => {
          setPromo(p);
          setPromoSource('deeplink');
        })
        .catch(() => {});
    fetchMe()
      .then((me) => {
        if (me.name) setName((v) => v || me.name!);
        if (me.phone) setPhone((v) => v || me.phone!);
        const pending = pendingPromo();
        if (pending) applyDeeplink(pending);
        else if (me.promo_code) applyDeeplink(me.promo_code);
      })
      .catch(() => {});
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (items.length === 0 && !submitting) navigate('/cart', { replace: true });
  }, [items.length, submitting, navigate]);

  const applyPromo = async () => {
    const code = promoInput.trim();
    if (!code) return;
    setPromoError('');
    try {
      const p = await checkPromo(code, orderItems);
      setPromo(p);
      setPromoSource('form');
      haptic('success');
    } catch (e) {
      setPromo(null);
      setPromoSource(null);
      setPromoError((e as Error).message);
    }
  };

  const discount = promo?.discount ?? 0;
  const discounted = Math.max(0, total - discount);
  const selectedDay = days?.find((d) => d.date === date);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      const order = await createOrder({
        items: orderItems,
        name,
        phone,
        delivery_address: address,
        delivery_date: date,
        delivery_time: slot,
        comment,
        card_text: cardText,
        is_anonymous: isAnonymous,
        recipient_name: notSelf ? recipientName : '',
        recipient_phone: notSelf ? recipientPhone : '',
        address_by_recipient: notSelf && addressByRecipient,
        // Промокод из deep-link бота сервер применит сам по initData.
        promo_code: promo ? promo.code : '',
        idempotency_key: idempotencyKey,
      });
      clear();
      clearPendingPromo();
      haptic('success');
      navigate(`/confirmation/${order.id}`, { state: { order }, replace: true });
    } catch (err) {
      setError((err as Error).message);
      setSubmitting(false);
    }
  };

  const addressRequired = !(notSelf && addressByRecipient);

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

        <Toggle checked={notSelf} onChange={setNotSelf} label={c.notSelf} />

        {notSelf && (
          <div className="animate-fade-in space-y-5">
            <Field label={c.recipientName}>
              <input
                className={inputCls}
                value={recipientName}
                onChange={(e) => setRecipientName(e.target.value)}
                placeholder={c.recipientNamePlaceholder}
                required
              />
            </Field>
            <Field label={c.recipientPhone}>
              <input
                className={inputCls}
                type="tel"
                inputMode="tel"
                value={recipientPhone}
                onChange={(e) => setRecipientPhone(e.target.value)}
                placeholder={c.phonePlaceholder}
                required
              />
            </Field>
            <Toggle checked={addressByRecipient} onChange={setAddressByRecipient} label={c.addressByRecipient} />
          </div>
        )}

        {addressRequired && (
          <Field label={c.address}>
            <input
              className={inputCls}
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              placeholder={c.addressPlaceholder}
              required
            />
          </Field>
        )}

        <Field label={c.date}>
          <div className="-mx-4 flex gap-2 overflow-x-auto px-4 pb-1 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
            {(days ?? []).map((d) => (
              <button
                type="button"
                key={d.date}
                onClick={() => {
                  haptic('light');
                  setDate(d.date);
                  setSlot('');
                }}
                className={`whitespace-nowrap rounded-button border px-4 py-2.5 font-mono text-[12px] font-bold uppercase tracking-wide transition-all duration-200 active:scale-[0.97] ${
                  date === d.date
                    ? 'neon-glow border-transparent bg-accent text-on-accent'
                    : 'border-line bg-surface text-muted'
                }`}
              >
                {d.label}
              </button>
            ))}
          </div>
        </Field>

        <Field label={c.time}>
          {selectedDay && (
            <div className="grid grid-cols-2 gap-2">
              {selectedDay.slots.map((s) => (
                <button
                  type="button"
                  key={s.slot}
                  disabled={!s.available}
                  onClick={() => {
                    haptic('light');
                    setSlot(s.slot);
                  }}
                  className={`rounded-input border py-3 font-mono text-[12px] font-bold tracking-wide transition-all duration-200 active:scale-[0.97] disabled:cursor-not-allowed disabled:opacity-40 ${
                    slot === s.slot
                      ? 'neon-glow border-transparent bg-accent text-on-accent'
                      : 'border-line bg-surface text-muted'
                  }`}
                >
                  <span className="block">{s.slot}</span>
                  {!s.available && <span className="block text-[10px] font-medium lowercase">{s.reason}</span>}
                </button>
              ))}
            </div>
          )}
          {days !== null && !selectedDay && (
            <p className="text-sm lowercase text-muted">{c.slotsEmpty}</p>
          )}
          <p className="mt-1.5 text-xs lowercase text-muted">{c.timeNote}</p>
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
            maxLength={200}
          />
          {cardText && (
            <p className="mt-1 text-xs text-muted">{cardText.length} / 200</p>
          )}
        </Field>

        <Toggle checked={isAnonymous} onChange={setIsAnonymous} label={c.anonymous} />

        <Field label={c.promo}>
          {promo ? (
            <div className="flex items-center justify-between rounded-input border border-accent/50 bg-surface px-4 py-3.5 shadow-[0_0_10px_rgba(128,255,0,0.12)]">
              <span className="flex items-center gap-2 font-mono text-[13px] font-bold uppercase">
                <span className="h-1.5 w-1.5 rounded-full bg-accent" />
                {promo.code} <span className="text-accent-2">−{promo.label}</span>
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
                  {c.discount} ({promo.code})
                </span>
                <span>−{formatPrice(discount)}</span>
              </div>
            </div>
          )}
          <div className="flex items-baseline justify-between">
            <span className="label">{c.total}</span>
            <span className="text-[28px] font-extrabold tracking-tight">{formatPrice(discounted)}</span>
          </div>
        </div>

        {error && <p className="text-center text-sm lowercase text-red-500">{error}</p>}

        <button
          type="submit"
          disabled={submitting || !date || !slot}
          className="btn-accent btn-accent-strong w-full rounded-button py-4 text-[15px] font-bold lowercase text-on-accent disabled:opacity-50"
        >
          {submitting ? c.submitting : c.submit}
        </button>
      </form>
    </div>
  );
}
