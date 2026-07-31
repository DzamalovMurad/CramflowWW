package repository

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// --- Отложенные уведомления (планировщик) ---

// ScheduleNotification ставит уведомление в очередь. Повторный вызов для той же
// пары (order_id, type) — no-op: уникальный индекс + ON CONFLICT DO NOTHING.
func (r *Repository) ScheduleNotification(orderID uint, typ string, dueAt time.Time) error {
	return r.DB.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&model.Notification{OrderID: orderID, Type: typ, DueAt: dueAt}).Error
}

// DueNotifications — неотправленные уведомления, чей срок наступил.
func (r *Repository) DueNotifications(now time.Time, limit int) ([]model.Notification, error) {
	var out []model.Notification
	err := r.DB.Where("sent_at IS NULL AND due_at <= ?", now).
		Order("due_at ASC").Limit(limit).Find(&out).Error
	return out, err
}

// ClaimNotification атомарно помечает уведомление отправленным.
// false — его уже забрал другой процесс (guard от дублей).
func (r *Repository) ClaimNotification(id uint, at time.Time) (bool, error) {
	res := r.DB.Model(&model.Notification{}).
		Where("id = ? AND sent_at IS NULL", id).
		Update("sent_at", at)
	return res.RowsAffected > 0, res.Error
}

// UnclaimNotification возвращает уведомление в очередь (отправка не удалась,
// но ошибка временная — следующий тик планировщика попробует снова).
func (r *Repository) UnclaimNotification(id uint) error {
	return r.DB.Model(&model.Notification{}).Where("id = ?", id).
		Update("sent_at", nil).Error
}

// OpenFeedback — последний вопрос об отзыве для клиента с неистёкшим окном
// ответа. Возвращается и уже отвеченный (responded_at != NULL): follow-up
// сообщения тоже пересылаются админам, но промокод выдаётся один раз.
func (r *Repository) OpenFeedback(telegramID int64, window time.Duration) (*model.Notification, error) {
	var n model.Notification
	err := r.DB.
		Joins("JOIN orders ON orders.id = notifications.order_id").
		Joins("JOIN users ON users.id = orders.user_id").
		Where("users.telegram_id = ? AND notifications.type = ? AND notifications.sent_at > ?",
			telegramID, model.NotificationFeedback, time.Now().Add(-window)).
		Order("notifications.sent_at DESC").
		First(&n).Error
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// MarkFeedbackResponded помечает отзыв полученным.
// false — уже ответили раньше (второй промокод не выдаём).
func (r *Repository) MarkFeedbackResponded(id uint, at time.Time) (bool, error) {
	res := r.DB.Model(&model.Notification{}).
		Where("id = ? AND responded_at IS NULL", id).
		Update("responded_at", at)
	return res.RowsAffected > 0, res.Error
}

// --- Фото букета ---

func (r *Repository) AddOrderPhoto(p *model.OrderPhoto) error {
	return r.DB.Create(p).Error
}

// --- Одноразовые промокоды ---

// Алфавит без похожих символов (0/O, 1/I/L), чтобы код легко вводился с телефона.
const promoAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CreateSingleUsePromo генерирует уникальный одноразовый промокод вида PREFIX-XXXX.
func (r *Repository) CreateSingleUsePromo(prefix string, percent int) (*model.PromoCode, error) {
	var lastErr error
	for range 5 {
		suffix := make([]byte, 4)
		for i := range suffix {
			suffix[i] = promoAlphabet[rand.IntN(len(promoAlphabet))]
		}
		p := &model.PromoCode{
			Code:            fmt.Sprintf("%s-%s", prefix, suffix),
			DiscountPercent: percent,
			MaxUses:         1,
		}
		if err := r.DB.Create(p).Error; err != nil {
			lastErr = err
			if isDuplicateErr(err) {
				continue // коллизия кода — пробуем другой суффикс
			}
			return nil, err
		}
		return p, nil
	}
	return nil, lastErr
}

// isDuplicateErr — нарушение уникального индекса Postgres (SQLSTATE 23505).
func isDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23505") || strings.Contains(s, "duplicate key")
}

// --- Разовые бонусы (промокод за подписку) ---

func (r *Repository) GetClaimedBonus(userID uint, typ string) (*model.ClaimedBonus, error) {
	var cb model.ClaimedBonus
	err := r.DB.Preload("PromoCode").
		Where("user_id = ? AND type = ?", userID, typ).First(&cb).Error
	if err != nil {
		return nil, err
	}
	return &cb, nil
}

// CreateClaimedBonus фиксирует выдачу бонуса. gorm.ErrDuplicatedKey (через
// isDuplicateErr) — бонус уже выдавали, второй раз нельзя.
func (r *Repository) CreateClaimedBonus(cb *model.ClaimedBonus) error {
	if err := r.DB.Create(cb).Error; err != nil {
		if isDuplicateErr(err) {
			return gorm.ErrDuplicatedKey
		}
		return err
	}
	return nil
}

// --- Источники трафика ---

// SetUserAcquisitionSource записывает first-touch источник — только если поле пустое.
func (r *Repository) SetUserAcquisitionSource(userID uint, source string) error {
	if source == "" {
		return nil
	}
	return r.DB.Model(&model.User{}).
		Where("id = ? AND (acquisition_source = '' OR acquisition_source IS NULL)", userID).
		Update("acquisition_source", source).Error
}
