package repository

import (
	"time"
)

// ExportRow — строка CSV-выгрузки заказов. Состав заказа собирается в одну
// строку прямо в SQL (string_agg), поэтому выгрузка любого объёма — это один
// запрос, а не «заказы + позиции для каждого заказа».
type ExportRow struct {
	ID              uint
	CreatedAt       time.Time
	DeliveryDate    string
	DeliveryTime    string
	Status          string
	Items           string
	ItemsTotal      int64 // сумма позиций до скидки
	TotalPrice      int64 // итог заказа
	PromoCode       string
	PromoPercent    int
	Source          string
	ClientName      string
	ClientPhone     string
	RecipientName   string
	RecipientPhone  string
	DeliveryAddress string
	IsAnonymous     bool
	CancelReason    string
}

// ExportOrders — заказы, созданные начиная с from (нулевое время = все).
// Порядок — по номеру заказа, чтобы выгрузки за разные периоды читались одинаково.
func (r *Repository) ExportOrders(from time.Time) ([]ExportRow, error) {
	var rows []ExportRow
	err := r.DB.Raw(`
		SELECT o.id,
		       o.created_at,
		       o.delivery_date,
		       o.delivery_time,
		       o.status,
		       COALESCE(i.items, '')        AS items,
		       COALESCE(i.items_total, 0)   AS items_total,
		       o.total_price,
		       COALESCE(p.code, '')             AS promo_code,
		       COALESCE(p.discount_percent, 0)  AS promo_percent,
		       COALESCE(NULLIF(o.source, ''), 'direct') AS source,
		       COALESCE(u.name, '')          AS client_name,
		       COALESCE(u.phone, '')         AS client_phone,
		       COALESCE(o.recipient_name, '')  AS recipient_name,
		       COALESCE(o.recipient_phone, '') AS recipient_phone,
		       o.delivery_address,
		       o.is_anonymous,
		       COALESCE(o.cancel_reason, '') AS cancel_reason
		FROM orders o
		JOIN users u ON u.id = o.user_id
		LEFT JOIN promo_codes p ON p.id = o.promo_code_id
		LEFT JOIN LATERAL (
			SELECT string_agg(
			           oi.product_name || ' (' || v.quantity || ' шт) x' || oi.quantity,
			           '; ' ORDER BY oi.id
			       )                                    AS items,
			       SUM(oi.price * oi.quantity)          AS items_total
			FROM order_items oi
			JOIN product_variants v ON v.id = oi.variant_id
			WHERE oi.order_id = o.id
		) i ON TRUE
		WHERE o.created_at >= ?
		ORDER BY o.id`, from).Scan(&rows).Error
	return rows, err
}
