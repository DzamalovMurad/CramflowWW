package repository

import (
	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// --- Promo codes ---

func (r *Repository) GetPromoByCode(code string) (*model.PromoCode, error) {
	var p model.PromoCode
	err := r.DB.Where("UPPER(code) = UPPER(?)", code).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) GetPromoByID(id uint) (*model.PromoCode, error) {
	var p model.PromoCode
	if err := r.DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) CreatePromo(p *model.PromoCode) error {
	return r.DB.Create(p).Error
}

// ListPromos — промокоды для админ-бота: активные первыми, новые сверху.
func (r *Repository) ListPromos(limit int) ([]model.PromoCode, error) {
	var promos []model.PromoCode
	err := r.DB.Order("is_active DESC, id DESC").Limit(limit).Find(&promos).Error
	return promos, err
}

func (r *Repository) SetPromoActive(code string, active bool) error {
	res := r.DB.Model(&model.PromoCode{}).
		Where("UPPER(code) = UPPER(?)", code).
		Update("is_active", active)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// BindPromoToUser делает код персональным: применить сможет только этот пользователь.
func (r *Repository) BindPromoToUser(promoID, userID uint) error {
	return r.DB.Model(&model.PromoCode{}).Where("id = ?", promoID).
		Update("bound_user_id", userID).Error
}

// CountPromoRedemptions — сколько раз пользователь уже применил код
// (для лимита max_uses_per_user; отменённые до сборки заказы не считаются —
// их записи удалены, см. ChangeOrderStatus).
func (r *Repository) CountPromoRedemptions(promoID, userID uint) (int64, error) {
	var n int64
	err := r.DB.Model(&model.PromoRedemption{}).
		Where("promo_code_id = ? AND user_id = ?", promoID, userID).
		Count(&n).Error
	return n, err
}

// CountUserOrders — заказы пользователя без отменённых (для first_order_only).
func (r *Repository) CountUserOrders(userID uint) (int64, error) {
	var n int64
	err := r.DB.Model(&model.Order{}).
		Where("user_id = ? AND status <> ?", userID, model.StatusCancelled).
		Count(&n).Error
	return n, err
}

// PromoStats — агрегаты по применениям кода для /promo list и /promo info.
type PromoStats struct {
	Redemptions   int64 // активных применений
	TotalDiscount int64 // выдано скидок, ₽
	Revenue       int64 // выручка заказов с кодом (без отменённых), ₽
}

// GetPromoStatsMap — статистика по кодам одним запросом (ключ — promo_code_id).
func (r *Repository) GetPromoStatsMap() (map[uint]PromoStats, error) {
	var rows []struct {
		PromoCodeID   uint
		Redemptions   int64
		TotalDiscount int64
		Revenue       int64
	}
	err := r.DB.Raw(`
		SELECT pr.promo_code_id,
		       COUNT(*)                          AS redemptions,
		       COALESCE(SUM(pr.amount_saved), 0) AS total_discount,
		       COALESCE(SUM(o.total_price) FILTER (WHERE o.status <> 'cancelled'), 0) AS revenue
		FROM promo_redemptions pr
		JOIN orders o ON o.id = pr.order_id
		GROUP BY pr.promo_code_id`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uint]PromoStats, len(rows))
	for _, row := range rows {
		out[row.PromoCodeID] = PromoStats{
			Redemptions:   row.Redemptions,
			TotalDiscount: row.TotalDiscount,
			Revenue:       row.Revenue,
		}
	}
	return out, nil
}
