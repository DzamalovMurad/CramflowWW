package repository

import (
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Отчёт /stats собирается тремя агрегатными запросами (сводка, топ товаров,
// каналы привлечения). Ни один из них не ходит в БД в цикле: всё, что нужно
// посчитать по строкам, считает Postgres.

// Периоды отчёта.
const (
	PeriodToday = "today"
	PeriodWeek  = "week"
	PeriodMonth = "month"
)

var PeriodLabels = map[string]string{
	PeriodToday: "Сегодня",
	PeriodWeek:  "Неделя",
	PeriodMonth: "Месяц",
}

var PeriodOrder = []string{PeriodToday, PeriodWeek, PeriodMonth}

// PeriodStart — начало периода. «Сегодня» — с полуночи, остальное — окно
// в 7/30 дней назад от текущего момента.
func PeriodStart(period string, now time.Time) time.Time {
	switch period {
	case PeriodWeek:
		return now.AddDate(0, 0, -7)
	case PeriodMonth:
		return now.AddDate(0, 0, -30)
	default:
		y, m, d := now.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	}
}

// StatsSummary — сводка периода. Выручка разделена: доставленные заказы —
// это уже деньги, всё остальное в работе — ещё нет.
type StatsSummary struct {
	DoneOrders    int64   // доставлено
	DoneRevenue   int64   // выручка по доставленным
	ActiveOrders  int64   // в работе (не доставлен и не отменён)
	ActiveRevenue int64   // сумма заказов в работе
	Cancelled     int64   // отменено
	AvgCheck      float64 // средний чек по доставленным
	NewUsers      int64   // новых клиентов за период
}

// TopProduct — строка топа товаров по проданным штукам.
type TopProduct struct {
	Name    string
	Qty     int64
	Revenue int64
}

// SourceStat — заказы и выручка по каналу привлечения.
type SourceStat struct {
	Source  string
	Orders  int64
	Revenue int64
}

// GetStatsSummary — вся сводка одним запросом. Отменённые заказы не попадают
// ни в выручку, ни в средний чек, но считаются отдельным счётчиком.
func (r *Repository) GetStatsSummary(from time.Time) (*StatsSummary, error) {
	var s StatsSummary
	err := r.DB.Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = ?)                                  AS done_orders,
			COALESCE(SUM(total_price) FILTER (WHERE status = ?), 0)             AS done_revenue,
			COUNT(*) FILTER (WHERE status NOT IN (?, ?))                        AS active_orders,
			COALESCE(SUM(total_price) FILTER (WHERE status NOT IN (?, ?)), 0)   AS active_revenue,
			COUNT(*) FILTER (WHERE status = ?)                                  AS cancelled,
			COALESCE(AVG(total_price) FILTER (WHERE status = ?), 0)             AS avg_check,
			(SELECT COUNT(*) FROM users WHERE created_at >= ?)                  AS new_users
		FROM orders
		WHERE created_at >= ?`,
		model.StatusDelivered,
		model.StatusDelivered,
		model.StatusDelivered, model.StatusCancelled,
		model.StatusDelivered, model.StatusCancelled,
		model.StatusCancelled,
		model.StatusDelivered,
		from,
		from,
	).Scan(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetTopProducts — топ товаров по количеству проданных штук за период.
// Имя берём из order_items.product_name — оно зафиксировано на момент заказа,
// поэтому отчёт не разъезжается после переименования или удаления товара.
func (r *Repository) GetTopProducts(from time.Time, limit int) ([]TopProduct, error) {
	var out []TopProduct
	err := r.DB.Raw(`
		SELECT oi.product_name              AS name,
		       SUM(oi.quantity)             AS qty,
		       SUM(oi.price * oi.quantity)  AS revenue
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE o.created_at >= ? AND o.status <> ?
		GROUP BY oi.product_name
		ORDER BY qty DESC, revenue DESC
		LIMIT ?`, from, model.StatusCancelled, limit).Scan(&out).Error
	return out, err
}

// GetSourceStats — заказы и выручка по каналам за период (без отменённых).
func (r *Repository) GetSourceStats(from time.Time) ([]SourceStat, error) {
	var out []SourceStat
	err := r.DB.Raw(`
		SELECT COALESCE(NULLIF(source, ''), ?) AS source,
		       COUNT(*)                        AS orders,
		       COALESCE(SUM(total_price), 0)   AS revenue
		FROM orders
		WHERE created_at >= ? AND status <> ?
		GROUP BY 1
		ORDER BY orders DESC`,
		model.SourceDirect, from, model.StatusCancelled).Scan(&out).Error
	return out, err
}

// GroupSmallSources сворачивает мелкие каналы в «другое»: показываем
// топ-keep каналов, всё, что меньше minShare от заказов, уходит в общую строку.
// Возвращает список для печати — «другое» всегда последним.
func GroupSmallSources(stats []SourceStat, keep int, minShare float64) []SourceStat {
	var total int64
	for _, s := range stats {
		total += s.Orders
	}
	if total == 0 {
		return nil
	}

	out := make([]SourceStat, 0, keep+1)
	other := SourceStat{Source: model.SourceOther}
	for i, s := range stats {
		small := float64(s.Orders)/float64(total) < minShare
		if i < keep && !small {
			out = append(out, s)
			continue
		}
		other.Orders += s.Orders
		other.Revenue += s.Revenue
	}
	if other.Orders > 0 {
		out = append(out, other)
	}
	return out
}
