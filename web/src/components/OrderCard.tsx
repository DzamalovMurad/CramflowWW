import { content, statusLabels } from '../content';
import { formatDate, formatPrice, type Order } from '../types';

/** Цвет метки статуса: отменённый — красный, доставленный — приглушённый. */
function statusTone(status: string): string {
  if (status === 'cancelled') return 'bg-red-500/10 text-red-500';
  if (status === 'delivered') return 'bg-tile text-muted';
  return 'bg-accent/15 text-accent-2';
}

/** Метка статуса заказа. Подписи берутся из content.ts, значения — с сервера. */
export function StatusBadge({ order }: { order: Order }) {
  const label = statusLabels[order.status] ?? order.status_label ?? order.status;
  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-bold lowercase ${statusTone(
        order.status,
      )}`}
    >
      {label}
    </span>
  );
}

/**
 * Карточка заказа: состав, доставка, итог. Общая для экрана подтверждения
 * и списка «мои заказы» — чтобы клиент видел одно и то же в обоих местах.
 */
export default function OrderCard({ order, compact }: { order: Order; compact?: boolean }) {
  return (
    <div className="rounded-card border border-line bg-surface p-5">
      <div className="flex items-center justify-between gap-3">
        <span className="text-[15px] font-bold lowercase">
          {content.order.number} #{order.id}
        </span>
        <StatusBadge order={order} />
      </div>

      {!compact && (
        <>
          <p className="label mb-2.5 mt-4">{content.order.composition}</p>
          <div className="space-y-2.5">
            {order.items.map((item) => (
              <div key={item.id} className="flex items-baseline justify-between gap-3 text-sm">
                <span className="lowercase">
                  {item.product_name} · {item.flowers_count} {content.product.flowersUnit}
                  {item.quantity > 1 && <span className="text-muted"> ×{item.quantity}</span>}
                </span>
                <span className="whitespace-nowrap font-mono font-medium">
                  {formatPrice(item.price * item.quantity)}
                </span>
              </div>
            ))}
          </div>

          {order.promo_code && (
            <p className="mt-3.5 flex flex-wrap items-center gap-x-2 text-sm lowercase text-accent-2">
              <span className="h-1.5 w-1.5 rounded-full bg-accent-2" />
              {content.order.promoApplied}:
              <span className="normal-case">{order.promo_code.code}</span>
              <span>(−{formatPrice(order.discount_amount)})</span>
            </p>
          )}
        </>
      )}

      <div className="mt-4 flex justify-between border-t border-line pt-3.5 font-bold lowercase">
        <span>{content.order.total}</span>
        <span className="font-mono">{formatPrice(order.total_price)}</span>
      </div>

      <p className="mt-3 text-xs lowercase leading-relaxed text-muted">
        {content.order.delivery}: {formatDate(order.delivery_date)}, {order.delivery_time}
        {!compact && ` · ${order.delivery_address}`}
      </p>

      {order.status === 'cancelled' && order.cancel_reason && (
        <p className="mt-2 text-xs lowercase leading-relaxed text-red-500">
          {content.order.cancelReason}: {order.cancel_reason}
        </p>
      )}
    </div>
  );
}
