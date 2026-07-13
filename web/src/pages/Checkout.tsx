import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import Header from '../components/Header';
import { checkPromo, createOrder, fetchMe } from '../api';
import { useCart } from '../cart';
import { haptic, tg } from '../telegram';
import { TIME_SLOTS, formatPrice } from '../types';

const inputCls =
  'w-full rounded-card border border-line bg-surface px-4 py-3 text-base outline-none transition-colors placeholder:text-muted focus:border-accent-2';

/** Оформление заказа: контакты, адрес, дата/время, комментарий, промокод. */
export default function Checkout() {
  const { items, total, clear } = useCart();
  const navigate = useNavigate();

  const [name, setName] = useState('');
  const [phone, setPhone] = useState('');
  const [address, setAddress] = useState('');
  const [date, setDate] = useState('');
  const [time, setTime] = useState('');
  const [comment, setComment] = useState('');

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

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      const order = await createOrder({
        items: items.map((i) => ({ variant_id: i.variantId, quantity: i.qty })),
        name,
        phone,
        delivery_address: address,
        delivery_date: date,
        delivery_time: time,
        comment,
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
      <Header title="Оформление" showBack={!tg()} />

      <form onSubmit={submit} className="space-y-4 p-4">
        <div>
          <label className="mb-1.5 block text-sm font-medium">Ваше имя</label>
          <input
            className={inputCls}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Иван"
            required
          />
        </div>

        <div>
          <label className="mb-1.5 block text-sm font-medium">Телефон</label>
          <input
            className={inputCls}
            type="tel"
            inputMode="tel"
            value={phone}
            onChange={(e) => setPhone(e.target.value)}
            placeholder="+7 900 000-00-00"
            required
          />
        </div>

        <div>
          <label className="mb-1.5 block text-sm font-medium">Адрес доставки</label>
          <input
            className={inputCls}
            value={address}
            onChange={(e) => setAddress(e.target.value)}
            placeholder="Улица, дом, квартира"
            required
          />
        </div>

        <div>
          <label className="mb-1.5 block text-sm font-medium">Дата доставки</label>
          <input
            className={inputCls}
            type="date"
            min={today}
            value={date}
            onChange={(e) => setDate(e.target.value)}
            required
          />
        </div>

        <div>
          <label className="mb-1.5 block text-sm font-medium">Время доставки</label>
          <div className="grid grid-cols-3 gap-2">
            {TIME_SLOTS.map((slot) => (
              <button
                type="button"
                key={slot}
                onClick={() => {
                  haptic('light');
                  setTime(slot);
                }}
                className={`rounded-card border py-3 text-sm font-medium transition-colors ${
                  time === slot ? 'border-accent bg-accent/10' : 'border-line bg-surface'
                }`}
              >
                {slot}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="mb-1.5 block text-sm font-medium">
            Комментарий <span className="font-normal text-muted">(необязательно)</span>
          </label>
          <textarea
            className={`${inputCls} resize-none`}
            rows={2}
            value={comment}
            onChange={(e) => setComment(e.target.value)}
            placeholder="Например: без сирени"
          />
        </div>

        <div>
          <label className="mb-1.5 block text-sm font-medium">Промокод</label>
          {promo ? (
            <div className="flex items-center justify-between rounded-card border border-accent-2 bg-accent/10 px-4 py-3">
              <span className="text-sm font-medium">
                🎁 {promo.code} — скидка {promo.discount_percent}%
              </span>
              {promoSource === 'form' && (
                <button
                  type="button"
                  className="text-sm text-muted"
                  onClick={() => {
                    setPromo(null);
                    setPromoSource(null);
                    setPromoInput('');
                  }}
                >
                  Убрать
                </button>
              )}
            </div>
          ) : (
            <div className="flex gap-2">
              <input
                className={inputCls}
                value={promoInput}
                onChange={(e) => setPromoInput(e.target.value.toUpperCase())}
                placeholder="WELCOME10"
              />
              <button
                type="button"
                onClick={applyPromo}
                className="whitespace-nowrap rounded-card border border-line px-4 text-sm font-medium active:bg-line"
              >
                Применить
              </button>
            </div>
          )}
          {promoError && <p className="mt-1 text-xs text-red-500">{promoError}</p>}
        </div>

        <div className="space-y-1 border-t border-line pt-4">
          {promo && (
            <>
              <div className="flex justify-between text-sm text-muted">
                <span>Сумма</span>
                <span>{formatPrice(total)}</span>
              </div>
              <div className="flex justify-between text-sm text-accent-2">
                <span>Скидка {promo.discount_percent}%</span>
                <span>−{formatPrice(total - discounted)}</span>
              </div>
            </>
          )}
          <div className="flex justify-between text-lg font-bold">
            <span>Итого</span>
            <span>{formatPrice(discounted)}</span>
          </div>
        </div>

        {error && <p className="text-center text-sm text-red-500">{error}</p>}

        <button
          type="submit"
          disabled={submitting || !time}
          className="w-full rounded-card bg-accent py-4 text-base font-semibold text-[#111111] shadow-card transition-transform active:scale-[0.98] disabled:opacity-50"
        >
          {submitting ? 'Отправляем…' : 'Подтвердить'}
        </button>
      </form>
    </div>
  );
}
