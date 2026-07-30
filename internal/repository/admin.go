package repository

import (
	"strings"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// --- Админ-CRM: заказы ---

// CountOrdersByStatus — счётчики для бейджей в фильтре статусов (один GROUP BY).
func (r *Repository) CountOrdersByStatus() (map[string]int64, error) {
	var rows []struct {
		Status string
		N      int64
	}
	if err := r.DB.Model(&model.Order{}).
		Select("status, COUNT(*) AS n").
		Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, row := range rows {
		out[row.Status] = row.N
	}
	return out, nil
}

// ListOrdersByStatusPage — страница заказов статуса (новые сверху).
func (r *Repository) ListOrdersByStatusPage(status string, page, per int) ([]model.Order, error) {
	if page < 1 {
		page = 1
	}
	var orders []model.Order
	err := r.DB.Preload("User").Preload("Items").
		Where("status = ?", status).
		Order("id DESC").
		Limit(per).Offset((page - 1) * per).
		Find(&orders).Error
	return orders, err
}

// ListPreorders — активные заказы с доставкой позже сегодняшней даты,
// отсортированы по дате и слоту.
func (r *Repository) ListPreorders(today string, limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.DB.Preload("User").Preload("Items").
		Where("delivery_date > ? AND status NOT IN ?", today,
			[]string{model.StatusDelivered, model.StatusCancelled}).
		Order("delivery_date ASC, delivery_time ASC, id ASC").
		Limit(limit).
		Find(&orders).Error
	return orders, err
}

// ChangeOrderStatus — смена статуса + запись в журнал одной транзакцией.
// При отмене до сборки (из new/confirmed) применение промокода снимается:
// запись promo_redemptions удаляется, used_count уменьшается — код снова доступен.
func (r *Repository) ChangeOrderStatus(orderID uint, from, to string, adminID int64, cancelReason string) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		upd := map[string]any{"status": to}
		if cancelReason != "" {
			upd["cancel_reason"] = cancelReason
		}
		res := tx.Model(&model.Order{}).
			Where("id = ? AND status = ?", orderID, from). // защита от гонки двух админов
			Updates(upd)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if to == model.StatusCancelled && (from == model.StatusNew || from == model.StatusConfirmed) {
			var red model.PromoRedemption
			if err := tx.Where("order_id = ?", orderID).First(&red).Error; err == nil {
				if err := tx.Delete(&red).Error; err != nil {
					return err
				}
				if err := tx.Model(&model.PromoCode{}).Where("id = ?", red.PromoCodeID).
					Update("used_count", gorm.Expr("GREATEST(used_count - 1, 0)")).Error; err != nil {
					return err
				}
			}
		}
		return tx.Create(&model.OrderStatusLog{
			OrderID:    orderID,
			FromStatus: from,
			ToStatus:   to,
			AdminID:    adminID,
		}).Error
	})
}

// --- Админ-CRM: клиенты ---

// SearchClients — поиск по имени/телефону, до limit результатов.
func (r *Repository) SearchClients(q string, limit int) ([]model.User, error) {
	q = strings.TrimSpace(q)
	var users []model.User
	err := r.DB.
		Where("name ILIKE ? OR phone ILIKE ?", "%"+q+"%", "%"+q+"%").
		Order("id DESC").Limit(limit).
		Find(&users).Error
	return users, err
}

// ClientStats — LTV, средний чек и счётчики одним агрегатным запросом.
type ClientStats struct {
	Orders    int64
	Delivered int64
	Cancelled int64
	LTV       int64
	AvgCheck  float64
}

func (r *Repository) GetClientStats(userID uint) (*ClientStats, error) {
	var s ClientStats
	err := r.DB.Raw(`
		SELECT COUNT(*)                                                    AS orders,
		       COUNT(*) FILTER (WHERE status = 'delivered')                AS delivered,
		       COUNT(*) FILTER (WHERE status = 'cancelled')                AS cancelled,
		       COALESCE(SUM(total_price) FILTER (WHERE status = 'delivered'), 0) AS ltv,
		       COALESCE(AVG(total_price) FILTER (WHERE status = 'delivered'), 0) AS avg_check
		FROM orders WHERE user_id = ?`, userID).Scan(&s).Error
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// LastClientOrders — последние заказы клиента для карточки.
func (r *Repository) LastClientOrders(userID uint, limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.DB.Where("user_id = ?", userID).
		Order("id DESC").Limit(limit).
		Find(&orders).Error
	return orders, err
}

func (r *Repository) GetUserByID(id uint) (*model.User, error) {
	var u model.User
	if err := r.DB.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}
