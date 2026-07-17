import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import Header from '../components/Header';
import { fetchMe } from '../api';
import { content } from '../content';

const c = content.profile;

interface Me {
  name?: string;
  phone?: string;
  promo_code?: string;
  discount_percent?: number;
}

/** Профиль: сохранённые имя/телефон и промокод из deep-link. */
export default function Profile() {
  const [me, setMe] = useState<Me | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    fetchMe()
      .then(setMe)
      .catch(() => {})
      .finally(() => setLoaded(true));
  }, []);

  const hasData = me && (me.name || me.phone || me.promo_code);

  return (
    <div className="pb-28">
      <Header title={c.title} />

      <div className="p-4">
        {loaded && !hasData && (
          <div className="animate-fade-up rounded-card bg-surface p-6 text-center shadow-card">
            <p className="text-[15px] font-bold">{c.guest}</p>
            <p className="mt-1.5 text-sm text-muted">{c.guestHint}</p>
            <Link
              to="/catalog"
              className="btn-accent mt-5 inline-block rounded-button px-6 py-3 text-[14px] font-bold lowercase"
            >
              {c.toCatalog}
            </Link>
          </div>
        )}

        {hasData && (
          <div className="animate-fade-up space-y-3">
            {me!.name && (
              <Row label={c.name} value={me!.name} />
            )}
            {me!.phone && <Row label={c.phone} value={me!.phone} />}
            {me!.promo_code && (
              <div className="rounded-card bg-ink p-5 text-page shadow-card">
                <p className="text-[11px] font-bold uppercase tracking-wider opacity-70">{c.promo}</p>
                <div className="mt-1 flex items-baseline justify-between">
                  <span className="text-[22px] font-extrabold text-accent">{me!.promo_code}</span>
                  <span className="text-sm opacity-80">
                    {c.discount} {me!.discount_percent}%
                  </span>
                </div>
              </div>
            )}
          </div>
        )}

        <div className="mt-6 rounded-card bg-surface p-5 shadow-card">
          <p className="label mb-2">{c.about}</p>
          <p className="text-sm leading-relaxed text-muted">{c.aboutText}</p>
        </div>
      </div>
    </div>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between rounded-card bg-surface px-5 py-4 shadow-card">
      <span className="label">{label}</span>
      <span className="text-[15px] font-bold">{value}</span>
    </div>
  );
}
