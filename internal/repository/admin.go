package repository

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// ─── Админ-CRM: заказы ─────────────────────────────────────────────────────

// CountOrdersByStatus — счётчики для бейджей в фильтре статусов (один GROUP BY).
func (r *Repository) CountOrdersByStatus(ctx context.Context) (map[string]int64, error) {
	var rows []struct {
		Status string
		N      int64
	}
	if err := r.db(ctx).Model(&model.Order{}).
		Select("status, COUNT(*) AS n").
		Group("status").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("счётчики заказов: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.N
	}
	return out, nil
}

// ListOrdersByStatusPage — страница заказов статуса (новые сверху).
func (r *Repository) ListOrdersByStatusPage(ctx context.Context, status string, page, per int) ([]model.Order, error) {
	if page < 1 {
		page = 1
	}
	var orders []model.Order
	err := r.db(ctx).Preload("User").
		Where("status = ?", status).
		Order("id DESC").
		Limit(per).Offset((page - 1) * per).
		Find(&orders).Error
	return orders, wrap(err)
}

// ListOrdersForDate — заказы с доставкой на указанную дату (кроме отменённых),
// по времени доставки. Основной утренний запрос флориста: «что сегодня везём».
func (r *Repository) ListOrdersForDate(ctx context.Context, date string, limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.db(ctx).Preload("User").
		Where("delivery_date = ? AND status <> ?", date, model.StatusCancelled).
		Order("delivery_time ASC, id ASC").
		Limit(limit).
		Find(&orders).Error
	return orders, wrap(err)
}

// ListPreorders — активные заказы с доставкой позже указанной даты.
func (r *Repository) ListPreorders(ctx context.Context, today string, limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.db(ctx).Preload("User").
		Where("delivery_date > ? AND status NOT IN ?", today,
			[]string{model.StatusDelivered, model.StatusCancelled}).
		Order("delivery_date ASC, delivery_time ASC, id ASC").
		Limit(limit).
		Find(&orders).Error
	return orders, wrap(err)
}

// ChangeOrderStatus — смена статуса + запись в журнал одной транзакцией.
// Условие `status = from` защищает от гонки двух админов: второй получит
// ErrNotFound и подсказку обновить карточку.
func (r *Repository) ChangeOrderStatus(ctx context.Context, orderID uint, from, to string, adminID int64, cancelReason string) error {
	upd := map[string]any{"status": to, "updated_at": gorm.Expr("NOW()")}
	if cancelReason != "" {
		upd["cancel_reason"] = cancelReason
	}
	res := r.db(ctx).Model(&model.Order{}).
		Where("id = ? AND status = ?", orderID, from).
		Updates(upd)
	if res.Error != nil {
		return fmt.Errorf("смена статуса: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	if err := r.db(ctx).Create(&model.OrderStatusLog{
		OrderID:    orderID,
		FromStatus: from,
		ToStatus:   to,
		AdminID:    adminID,
	}).Error; err != nil {
		return fmt.Errorf("журнал статусов: %w", err)
	}
	return nil
}

// ─── Админ-CRM: клиенты ────────────────────────────────────────────────────

// SearchClients — поиск по имени/телефону, до limit результатов.
// LIKE-шаблон экранируется: иначе «%» в запросе выбирает всю базу.
func (r *Repository) SearchClients(ctx context.Context, q string, limit int) ([]model.User, error) {
	pattern := "%" + escapeLike(strings.TrimSpace(q)) + "%"
	var users []model.User
	err := r.db(ctx).
		Where(`name ILIKE @p ESCAPE '\' OR phone ILIKE @p ESCAPE '\'`,
			map[string]any{"p": pattern}).
		Order("id DESC").Limit(limit).
		Find(&users).Error
	return users, wrap(err)
}

// escapeLike обезвреживает спецсимволы шаблона LIKE.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// ClientStats — LTV, средний чек и счётчики одним агрегатным запросом.
type ClientStats struct {
	Orders    int64
	Delivered int64
	Cancelled int64
	LTV       int64
	AvgCheck  float64
}

func (r *Repository) GetClientStats(ctx context.Context, userID uint) (*ClientStats, error) {
	var s ClientStats
	err := r.db(ctx).Raw(`
		SELECT COUNT(*)                                                          AS orders,
		       COUNT(*) FILTER (WHERE status = ?)                                AS delivered,
		       COUNT(*) FILTER (WHERE status = ?)                                AS cancelled,
		       COALESCE(SUM(total_price) FILTER (WHERE status = ?), 0)           AS ltv,
		       COALESCE(AVG(total_price) FILTER (WHERE status = ?), 0)           AS avg_check
		FROM orders WHERE user_id = ?`,
		model.StatusDelivered, model.StatusCancelled,
		model.StatusDelivered, model.StatusDelivered, userID).Scan(&s).Error
	if err != nil {
		return nil, fmt.Errorf("статистика клиента: %w", err)
	}
	return &s, nil
}

// LastClientOrders — последние заказы клиента для карточки.
func (r *Repository) LastClientOrders(ctx context.Context, userID uint, limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.db(ctx).Where("user_id = ?", userID).
		Order("id DESC").Limit(limit).
		Find(&orders).Error
	return orders, wrap(err)
}

func (r *Repository) GetUserByID(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	if err := r.db(ctx).First(&u, id).Error; err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}
