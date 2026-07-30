package service

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// ValidatePromo — полная серверная проверка промокода для пользователя и корзины:
// активность, даты, лимиты (общий и на пользователя), минимальная сумма,
// область действия, «только первый заказ», персональная привязка.
// Возвращает промокод и размер скидки в рублях. Все отказы — ValidationError
// с понятным русским текстом правила.
func (s *Service) ValidatePromo(code string, telegramID int64, inputs []OrderItemInput) (*model.PromoCode, int, error) {
	priced, total, err := s.buildItems(inputs)
	if err != nil {
		return nil, 0, err
	}
	promo, err := s.Repo.GetPromoByCode(code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, invalid("Промокод не найден")
		}
		return nil, 0, err
	}
	// Пользователь может ещё не существовать (первый визит) — это не ошибка.
	user, err := s.Repo.GetUserByTelegramID(telegramID)
	if err != nil {
		user = nil
	}
	discount, err := s.checkPromoRules(promo, user, priced, total)
	if err != nil {
		return nil, 0, err
	}
	return promo, discount, nil
}

// checkPromoRules применяет правила по одному, чтобы каждая причина отказа
// имела свой текст. Гонки (двое одновременно тратят последнее использование)
// закрывает не эта проверка, а транзакция Repository.CreateOrderChecked.
func (s *Service) checkPromoRules(promo *model.PromoCode, user *model.User, priced []pricedItem, total int) (int, error) {
	now := time.Now()
	if !promo.IsActive {
		return 0, invalid("Промокод отключён")
	}
	if promo.StartsAt != nil && now.Before(*promo.StartsAt) {
		return 0, invalid("Промокод ещё не начал действовать")
	}
	if promo.ExpiresAt != nil && now.After(*promo.ExpiresAt) {
		return 0, invalid("Срок действия промокода истёк")
	}
	if promo.MaxUses > 0 && promo.UsedCount >= promo.MaxUses {
		return 0, invalid("Лимит использований промокода исчерпан")
	}
	if promo.BoundUserID != nil && (user == nil || user.ID != *promo.BoundUserID) {
		return 0, invalid("Это персональный промокод — он привязан к другому пользователю")
	}
	if user != nil {
		if promo.FirstOrderOnly {
			n, err := s.Repo.CountUserOrders(user.ID)
			if err != nil {
				return 0, err
			}
			if n > 0 {
				return 0, invalid("Промокод только для первого заказа")
			}
		}
		if promo.MaxUsesPerUser > 0 {
			n, err := s.Repo.CountPromoRedemptions(promo.ID, user.ID)
			if err != nil {
				return 0, err
			}
			if n >= int64(promo.MaxUsesPerUser) {
				return 0, invalid("Вы уже использовали этот промокод")
			}
		}
	}
	if promo.MinOrderAmount > 0 && total < promo.MinOrderAmount {
		return 0, invalid("Промокод действует для заказов от %d ₽", promo.MinOrderAmount)
	}
	eligible := eligibleAmount(promo, priced)
	if eligible == 0 {
		return 0, invalid("Промокод не действует на товары в корзине")
	}
	return promo.Discount(eligible), nil
}

// eligibleAmount — сумма позиций, на которые распространяется промокод
// (applies_to: all | category:<имя> | products:<id,id,…>). Скидка считается
// только от суммы товаров: доставка (если появится платная) не дисконтируется.
func eligibleAmount(promo *model.PromoCode, priced []pricedItem) int {
	scope := strings.TrimSpace(promo.AppliesTo)
	sum := 0
	switch {
	case scope == "" || scope == model.PromoAppliesAll:
		for _, pi := range priced {
			sum += pi.item.Price * pi.item.Quantity
		}
	case strings.HasPrefix(scope, "category:"):
		category := strings.TrimPrefix(scope, "category:")
		for _, pi := range priced {
			if pi.product.Category == category {
				sum += pi.item.Price * pi.item.Quantity
			}
		}
	case strings.HasPrefix(scope, "products:"):
		ids := map[uint]bool{}
		for _, part := range strings.Split(strings.TrimPrefix(scope, "products:"), ",") {
			if id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32); err == nil {
				ids[uint(id)] = true
			}
		}
		for _, pi := range priced {
			if ids[pi.product.ID] {
				sum += pi.item.Price * pi.item.Quantity
			}
		}
	}
	return sum
}
