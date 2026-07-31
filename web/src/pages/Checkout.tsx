import { useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import {
  ApiError,
  checkPromo,
  createOrder,
  fetchConfig,
  fetchMe,
  invalidateCatalog,
  newIdempotencyKey,
} from '../api';
import { useCart } from '../cart';
import { haptic, tg } from '../telegram';
import { content, fill } from '../content';
import { formatDate, formatHour, formatPrice, shiftDate, type ShopConfig } from '../types';
import { IconCheck } from '../components/icons';

const c = content.checkout;

const inputBase =
  'w-full min-h-[48px] rounded-input border bg-surface px-4 py-3 text-[15px] text-ink outline-none transition-all duration-200 placeholder:text-muted';
const inputOk = `${inputBase} border-line focus:border-accent focus:shadow-[0_0_10px_rgba(128,255,0,0.2)]`;
const inputBad = `${inputBase} border-red-500 focus:border-red-500`;

type DateMode = 'today' | 'tomorrow' | 'other';
type TimeMode = 'express' | 'at';
type Field = 'name' | 'phone' | 'address' | 'date' | 'time' | 'recipientName' | 'recipientPhone';

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="space-y-4">
      <h2 className="label">{title}</h2>
      {children}
    </section>
  );
}

function Field({
  label,
  optional,
  error,
  children,
}: {
  label: string;
  optional?: string;
  error?: string;
  children: ReactNode;
}) {
  return (
    <div>
      <label className="label mb-1.5 block">
        {label}
        {optional && <span className="text-muted"> · {optional}</span>}
      </label>
      {children}
      {error && <p className="mt-1.5 text-xs lowercase text-red-500">{error}</p>}
    </div>
  );
}

/** Оформление заказа: контакты, доставка, необязательные детали. */
export default function Checkout() {
  const { items, total, clear, removeMany } = useCart();
  const navigate = useNavigate();

  const [cfg, setCfg] = useState<ShopConfig | null>(null);
  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  const [address, setAddress] = useState('');

  const [forOther, setForOther] = useState(false);
  const [recipientName, setRecipientName] = useState('');
  const [recipientPhone, setRecipientPhone] = useState('');

  const [dateMode, setDateMode] = useState<DateMode>('today');
  const [customDate, setCustomDate] = useState('');
  const [timeMode, setTimeMode] = useState<TimeMode>('express');
  const [timeAt, setTimeAt] = useState('');

  const [showExtras, setShowExtras] = useState(false);
  const [comment, setComment] = useState('');
  const [cardText, setCardText] = useState('');
  const [isAnonymous, setIsAnonymous] = useState(false);

  const [promoInput, setPromoInput] = useState('');
  const [promo, setPromo] = useState<{ code: string; discount_percent: number } | null>(null);
  const [promoError, setPromoError] = useState('');
  const [promoSource, setPromoSource] = useState<'form' | 'deeplink' | null>(null);

  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [errors, setErrors] = useState<Partial<Record<Field, string>>>({});
  const [ordered, setOrdered] = useState(false);

  // Один ключ на всю попытку оформления: повтор после обрыва сети вернёт
  // уже созданный заказ, а не создаст второй.
  const idempotencyKey = useRef(newIdempotencyKey());

  // Правила доставки — с сервера, чтобы форма не предлагала невозможного.
  useEffect(() => {
    fetchConfig()
      .then((data) => {
        setCfg(data);
        if (!data.express_available) {
          setTimeMode('at');
          // Если сегодня уже не успеть — сразу предлагаем завтра.
          if (!data.earliest_today) setDateMode('tomorrow');
        }
      })
      .catch(() => {
        // Без конфига форма всё равно работает: сервер проверит время сам.
      });
  }, []);

  // Подтягиваем контакты прошлых заказов, имя из профиля Telegram
  // и промокод, полученный по deep-link.
  useEffect(() => {
    fetchMe()
      .then((me) => {
        setName((v) => v || me.name || me.telegram_name || '');
        setPhone((v) => v || me.phone || '');
        if (me.promo_code && me.discount_percent) {
          setPromo({ code: me.promo_code, discount_percent: me.discount_percent });
          setPromoSource('deeplink');
        }
      })
      .catch(() => {});
  }, []);

  // Пустая корзина на этом экране — тупик. Но после успешного заказа
  // корзина тоже пуста, и тогда уводить обратно нельзя.
  useEffect(() => {
    if (items.length === 0 && !submitting && !ordered) navigate('/cart', { replace: true });
  }, [items.length, submitting, ordered, navigate]);

  const today = cfg?.today ?? new Date().toISOString().slice(0, 10);
  const tomorrow = shiftDate(today, 1);
  const maxDate = shiftDate(today, cfg?.max_preorder_days ?? 60);

  const date = dateMode === 'today' ? today : dateMode === 'tomorrow' ? tomorrow : customDate;
  const isToday = date === today;

  // Экспресс возможен только сегодня и только в рабочие часы.
  const expressAvailable = isToday && (cfg?.express_available ?? true);
  const deliveryTime = timeMode === 'express' ? c.timeExpress : timeAt ? `к ${timeAt}` : '';

  // Дата больше не сегодня → экспресс невозможен, переключаем на «ко времени».
  useEffect(() => {
    if (timeMode === 'express' && !expressAvailable) setTimeMode('at');
  }, [expressAvailable, timeMode]);

  const minTime = useMemo(() => {
    if (!cfg) return '09:00';
    if (isToday && cfg.earliest_today) return cfg.earliest_today.padStart(5, '0');
    return `${String(cfg.open_hour).padStart(2, '0')}:00`;
  }, [cfg, isToday]);

  const maxTime = cfg ? `${String(cfg.close_hour).padStart(2, '0')}:00` : '21:00';

  const discounted = promo ? Math.floor((total * (100 - promo.discount_percent)) / 100) : total;

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

  /** Проверки на клиенте — чтобы не гонять человека на сервер за очевидным.
   *  Настоящая валидация всё равно на сервере. */
  const validate = (): boolean => {
    const next: Partial<Record<Field, string>> = {};
    if (name.trim().length < 2) next.name = 'укажите имя';
    if (phone.replace(/\D/g, '').length < 10) next.phone = 'укажите телефон полностью';
    if (address.trim().length < 5) next.address = 'укажите адрес: улица, дом, квартира';
    if (!date) next.date = 'выберите дату';
    if (!deliveryTime) next.time = 'выберите время';
    if (forOther) {
      if (recipientName.trim().length < 2) next.recipientName = 'укажите имя получателя';
      if (recipientPhone.replace(/\D/g, '').length < 10) {
        next.recipientPhone = 'укажите телефон получателя';
      }
    }
    setErrors(next);
    if (Object.keys(next).length > 0) {
      setError(c.fixErrors);
      haptic('medium');
      return false;
    }
    setError('');
    return true;
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (submitting) return;
    if (!validate()) return;

    setSubmitting(true);
    try {
      const order = await createOrder(
        {
          items: items.map((i) => ({ variant_id: i.variantId, quantity: i.qty })),
          name: name.trim(),
          phone: phone.trim(),
          delivery_address: address.trim(),
          delivery_date: date,
          delivery_time: deliveryTime,
          recipient_name: forOther ? recipientName.trim() : '',
          recipient_phone: forOther ? recipientPhone.trim() : '',
          comment: comment.trim(),
          card_text: cardText.trim(),
          is_anonymous: isAnonymous,
          // Промокод из deep-link сервер применит сам по initData.
          promo_code: promoSource === 'form' && promo ? promo.code : '',
        },
        idempotencyKey.current,
      );
      setOrdered(true);
      clear();
      invalidateCatalog(); // остатки изменились
      haptic('success');
      navigate(`/confirmation/${order.id}`, { state: { order }, replace: true });
    } catch (err) {
      const api = err as ApiError;
      // Часть букетов раскупили, пока клиент заполнял форму: убираем именно их
      // и оставляем человека в корзине с понятным объяснением.
      if (api.unavailableVariantIds?.length) {
        removeMany(api.unavailableVariantIds);
        navigate('/cart', { replace: true, state: { notice: content.cart.removed } });
        return;
      }
      setError(api.message);
      setSubmitting(false);
      haptic('medium');
    }
  };

  return (
    <div className="pb-40">
      <Header title={c.title} showBack={!tg()} />

      <form onSubmit={submit} noValidate className="space-y-7 p-4 pt-5">
        <Section title={c.contactsSection}>
          <Field label={c.name} error={errors.name}>
            <input
              className={errors.name ? inputBad : inputOk}
              value={name}
              autoComplete="name"
              onChange={(e) => setName(e.target.value)}
              placeholder={c.namePlaceholder}
              maxLength={80}
            />
          </Field>

          <Field label={c.phone} error={errors.phone}>
            <input
              className={errors.phone ? inputBad : inputOk}
              type="tel"
              inputMode="tel"
              autoComplete="tel"
              value={phone}
              onChange={(e) => setPhone(e.target.value)}
              placeholder={c.phonePlaceholder}
              maxLength={20}
            />
          </Field>
        </Section>

        <Section title={c.deliverySection}>
          <Field label={c.address} error={errors.address}>
            <input
              className={errors.address ? inputBad : inputOk}
              value={address}
              autoComplete="street-address"
              onChange={(e) => setAddress(e.target.value)}
              placeholder={c.addressPlaceholder}
              maxLength={300}
            />
          </Field>

          {/* Получатель. Курьер должен звонить тому, кто открывает дверь,
              иначе сюрприз испорчен, а доставка срывается. */}
          <Toggle
            checked={forOther}
            onChange={setForOther}
            label={c.forSomeoneElse}
            hint={c.forSomeoneElseHint}
          />
          {forOther && (
            <div className="animate-fade-in space-y-4">
              <Field label={c.recipientName} error={errors.recipientName}>
                <input
                  className={errors.recipientName ? inputBad : inputOk}
                  value={recipientName}
                  onChange={(e) => setRecipientName(e.target.value)}
                  placeholder={c.recipientNamePlaceholder}
                  maxLength={80}
                />
              </Field>
              <Field label={c.recipientPhone} error={errors.recipientPhone}>
                <input
                  className={errors.recipientPhone ? inputBad : inputOk}
                  type="tel"
                  inputMode="tel"
                  value={recipientPhone}
                  onChange={(e) => setRecipientPhone(e.target.value)}
                  placeholder={c.phonePlaceholder}
                  maxLength={20}
                />
              </Field>
            </div>
          )}

          <Field label={c.date} error={errors.date}>
            <div className="grid grid-cols-3 gap-2">
              {(
                [
                  ['today', `${c.dateToday}`],
                  ['tomorrow', c.dateTomorrow],
                  ['other', c.dateOther],
                ] as [DateMode, string][]
              ).map(([mode, label]) => (
                <Choice
                  key={mode}
                  active={dateMode === mode}
                  onClick={() => {
                    setDateMode(mode);
                    if (mode === 'other' && !customDate) setCustomDate(shiftDate(today, 2));
                  }}
                >
                  {label}
                </Choice>
              ))}
            </div>
            {dateMode === 'other' && (
              <input
                className={`${inputOk} animate-fade-in mt-2`}
                type="date"
                min={tomorrow}
                max={maxDate}
                value={customDate}
                onChange={(e) => setCustomDate(e.target.value)}
              />
            )}
            {dateMode !== 'other' && (
              <p className="mt-1.5 text-xs lowercase text-muted">{formatDate(date)}</p>
            )}
          </Field>

          <Field label={c.time} error={errors.time}>
            <div className="grid grid-cols-2 gap-2">
              <Choice
                active={timeMode === 'express'}
                disabled={!expressAvailable}
                onClick={() => setTimeMode('express')}
              >
                {c.timeExpress}
              </Choice>
              <Choice active={timeMode === 'at'} onClick={() => setTimeMode('at')}>
                {c.timeAt}
              </Choice>
            </div>
            {timeMode === 'at' && (
              <input
                className={`${inputOk} animate-fade-in mt-2`}
                type="time"
                min={minTime}
                max={maxTime}
                step={900}
                value={timeAt}
                onChange={(e) => setTimeAt(e.target.value)}
              />
            )}
            <p className="mt-1.5 text-xs lowercase text-muted">
              {isToday && !expressAvailable && !cfg?.earliest_today
                ? c.expressUnavailable
                : fill(c.timeNote, {
                    open: formatHour(cfg?.open_hour ?? 9),
                    close: formatHour(cfg?.close_hour ?? 21),
                  })}
            </p>
          </Field>
        </Section>

        {/* Всё необязательное — под одним раскрытием. Форма из пяти полей
            вместо девяти: меньше поводов бросить оформление. */}
        <section>
          <button
            type="button"
            onClick={() => {
              haptic('light');
              setShowExtras((v) => !v);
            }}
            aria-expanded={showExtras}
            className="flex min-h-[48px] w-full items-center justify-between rounded-input border border-line bg-surface px-4"
          >
            <span className="text-left">
              <span className="block text-[15px] lowercase text-ink">{c.extrasSection}</span>
              <span className="block text-xs lowercase text-muted">{c.extrasHint}</span>
            </span>
            <span className={`text-muted transition-transform ${showExtras ? 'rotate-180' : ''}`}>
              ▾
            </span>
          </button>

          {showExtras && (
            <div className="animate-fade-in mt-4 space-y-4">
              <Field label={c.cardText} optional={c.cardTextOptional}>
                <textarea
                  className={`${inputOk} resize-none`}
                  rows={2}
                  value={cardText}
                  onChange={(e) => setCardText(e.target.value)}
                  placeholder={c.cardTextPlaceholder}
                  maxLength={300}
                />
                {cardText && <p className="mt-1 text-xs text-muted">{cardText.length} / 300</p>}
              </Field>

              <Field label={c.comment} optional={c.commentOptional}>
                <textarea
                  className={`${inputOk} resize-none`}
                  rows={2}
                  value={comment}
                  onChange={(e) => setComment(e.target.value)}
                  placeholder={c.commentPlaceholder}
                  maxLength={500}
                />
              </Field>

              <Toggle checked={isAnonymous} onChange={setIsAnonymous} label={c.anonymous} />

              <Field label={c.promo}>
                {promo ? (
                  <div className="flex min-h-[48px] items-center justify-between rounded-input border border-accent/50 bg-surface px-4 shadow-[0_0_10px_rgba(128,255,0,0.12)]">
                    <span className="flex items-center gap-2 font-mono text-[13px] font-bold uppercase">
                      <span className="h-1.5 w-1.5 rounded-full bg-accent" />
                      {promo.code} <span className="text-accent-2">−{promo.discount_percent}%</span>
                    </span>
                    {promoSource === 'form' && (
                      <button
                        type="button"
                        className="-mr-2 min-h-[44px] px-2 text-sm lowercase text-muted"
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
                      className={inputOk}
                      value={promoInput}
                      onChange={(e) => setPromoInput(e.target.value.toUpperCase())}
                      placeholder={c.promoPlaceholder}
                      maxLength={32}
                    />
                    <button
                      type="button"
                      onClick={applyPromo}
                      className="min-h-[48px] whitespace-nowrap rounded-input border border-line bg-surface px-4 font-mono text-[12px] font-bold uppercase tracking-wide text-ink transition-transform active:scale-95"
                    >
                      {c.promoApply}
                    </button>
                  </div>
                )}
                {promoError && <p className="mt-1.5 text-xs lowercase text-red-500">{promoError}</p>}
              </Field>
            </div>
          )}
        </section>

        {promo && (
          <div className="rounded-card border border-line bg-surface p-4">
            <div className="flex justify-between font-mono text-[13px] uppercase text-muted">
              <span>{c.subtotal}</span>
              <span>{formatPrice(total)}</span>
            </div>
            <div className="mt-1.5 flex justify-between font-mono text-[13px] uppercase text-accent-2">
              <span>
                {c.discount} {promo.discount_percent}%
              </span>
              <span>−{formatPrice(total - discounted)}</span>
            </div>
          </div>
        )}
      </form>

      {/* Кнопка подтверждения всегда на виду: длинная форма не должна
          прятать главное действие в конце прокрутки. */}
      <div className="pb-safe fixed bottom-0 left-1/2 z-20 w-full max-w-md -translate-x-1/2 border-t border-line bg-page/95 px-4 pt-3 backdrop-blur">
        {error && (
          <p className="mb-2 text-center text-sm lowercase text-red-500" role="alert">
            {error}
          </p>
        )}
        <div className="mb-2.5 flex items-baseline justify-between">
          <span className="label">{c.total}</span>
          <span className="text-[24px] font-extrabold tracking-tight">
            {formatPrice(discounted)}
          </span>
        </div>
        <button
          type="button"
          onClick={submit}
          disabled={submitting}
          className="btn-accent btn-accent-strong min-h-[52px] w-full rounded-button text-[15px] font-bold lowercase text-on-accent disabled:opacity-50"
        >
          {submitting ? c.submitting : error ? c.submitRetry : c.submit}
        </button>
      </div>
    </div>
  );
}

/** Кнопка-выбор в группе (дата, время). */
function Choice({
  active,
  disabled,
  onClick,
  children,
}: {
  active: boolean;
  disabled?: boolean;
  onClick: () => void;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={() => {
        haptic('light');
        onClick();
      }}
      className={`min-h-[48px] rounded-input border px-2 text-[13px] font-bold lowercase transition-all duration-200 active:scale-[0.97] disabled:opacity-35 ${
        active
          ? 'neon-glow border-transparent bg-accent text-on-accent'
          : 'border-line bg-surface text-muted'
      }`}
    >
      {children}
    </button>
  );
}

/** Переключатель с кастомным чекбоксом и тач-зоной во всю строку. */
function Toggle({
  checked,
  onChange,
  label,
  hint,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  label: string;
  hint?: string;
}) {
  return (
    <label className="flex min-h-[52px] cursor-pointer items-center gap-3 rounded-input border border-line bg-surface px-4 py-3">
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
        className={`flex h-[24px] w-[24px] flex-shrink-0 items-center justify-center rounded-md border-2 transition-all duration-200 ${
          checked ? 'neon-glow border-transparent bg-accent' : 'border-line bg-transparent'
        }`}
      >
        <span
          className={`text-on-accent transition-transform duration-200 ${
            checked ? 'scale-100' : 'scale-0'
          }`}
        >
          <IconCheck size={14} />
        </span>
      </span>
      <span className="min-w-0">
        <span className="block text-[15px] lowercase text-ink">{label}</span>
        {hint && <span className="block text-xs lowercase leading-snug text-muted">{hint}</span>}
      </span>
    </label>
  );
}
