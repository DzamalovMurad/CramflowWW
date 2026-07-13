// Package service — бизнес-логика: расчёт сумм заказа и промокоды.
package service

import (
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

var DeliverySlots = []string{"10:00-12:00", "12:00-15:00", "15:00-18:00"}

type Service struct {
	Repo *repository.Repository
	// NotifyNewOrder вызывается после сохранения заказа (бот шлёт сообщение админу).
	NotifyNewOrder func(o *model.Order)
}

func New(repo *repository.Repository) *Service {
	return &Service{Repo: repo}
}

type OrderItemInput struct {
	VariantID uint `json:"variant_id"`
	Quantity  int  `json:"quantity"`
}

type OrderInput struct {
	Items           []OrderItemInput `json:"items"`
	Name            string           `json:"name"`
	Phone           string           `json:"phone"`
	DeliveryAddress string           `json:"delivery_address"`
	DeliveryDate    string           `json:"delivery_date"`
	DeliveryTime    string           `json:"delivery_time"`
	Comment         string           `json:"comment"`
	PromoCode       string           `json:"promo_code"`
	// TelegramID заполняется хендлером из initData, не клиентом.
	TelegramID int64 `json:"-"`
}

type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// CreateOrder валидирует вход, считает сумму по ценам из БД, применяет промокод
// (переданный явно или сохранённый у пользователя по deep-link) и сохраняет заказ.
func (s *Service) CreateOrder(in OrderInput) (*model.Order, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	in.DeliveryAddress = strings.TrimSpace(in.DeliveryAddress)
	in.DeliveryDate = strings.TrimSpace(in.DeliveryDate)
	in.Comment = strings.TrimSpace(in.Comment)

	if len(in.Items) == 0 {
		return nil, invalid("корзина пуста")
	}
	if in.Name == "" {
		return nil, invalid("укажите имя")
	}
	if len(in.Phone) < 6 {
		return nil, invalid("укажите корректный телефон")
	}
	if in.DeliveryAddress == "" {
		return nil, invalid("укажите адрес доставки")
	}
	if in.DeliveryDate == "" {
		return nil, invalid("укажите дату доставки")
	}
	validSlot := false
	for _, slot := range DeliverySlots {
		if in.DeliveryTime == slot {
			validSlot = true
			break
		}
	}
	if !validSlot {
		return nil, invalid("выберите время доставки")
	}

	user, err := s.Repo.UpsertUser(in.TelegramID, in.Name, in.Phone)
	if err != nil {
		return nil, err
	}

	// Собираем позиции по ценам из БД — клиентским ценам не доверяем.
	total := 0
	items := make([]model.OrderItem, 0, len(in.Items))
	for _, it := range in.Items {
		if it.Quantity < 1 || it.Quantity > 99 {
			return nil, invalid("некорректное количество")
		}
		variant, err := s.Repo.GetVariant(it.VariantID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, invalid("товар из корзины больше недоступен")
			}
			return nil, err
		}
		product, err := s.Repo.GetProduct(variant.ProductID)
		if err != nil || product.IsHidden {
			return nil, invalid("товар из корзины больше недоступен")
		}
		total += variant.Price * it.Quantity
		items = append(items, model.OrderItem{
			VariantID:   variant.ID,
			Quantity:    it.Quantity,
			Price:       variant.Price,
			ProductName: product.Name,
		})
	}

	// Промокод: явный из checkout приоритетнее сохранённого по deep-link.
	var promo *model.PromoCode
	if code := strings.TrimSpace(in.PromoCode); code != "" {
		promo, err = s.Repo.GetPromoByCode(code)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, invalid("промокод не найден")
			}
			return nil, err
		}
	} else if user.PromoCodeID != nil {
		promo, _ = s.Repo.GetPromoByID(*user.PromoCodeID)
	}

	var promoID *uint
	if promo != nil {
		total = total * (100 - promo.DiscountPercent) / 100
		promoID = &promo.ID
	}

	order := &model.Order{
		UserID:          user.ID,
		TotalPrice:      total,
		DeliveryAddress: in.DeliveryAddress,
		DeliveryDate:    in.DeliveryDate,
		DeliveryTime:    in.DeliveryTime,
		PromoCodeID:     promoID,
		Comment:         in.Comment,
		Status:          model.StatusNew,
		Items:           items,
	}
	if err := s.Repo.CreateOrder(order); err != nil {
		return nil, err
	}
	if promo != nil {
		_ = s.Repo.IncrementPromoUses(promo.ID)
	}

	full, err := s.Repo.GetOrder(order.ID)
	if err != nil {
		return nil, err
	}
	if s.NotifyNewOrder != nil {
		go s.NotifyNewOrder(full)
	}
	return full, nil
}

// ApplyDeepLinkPromo сохраняет промокод у пользователя (deep-link t.me/bot?start=CODE).
func (s *Service) ApplyDeepLinkPromo(telegramID int64, code string) (*model.PromoCode, error) {
	promo, err := s.Repo.GetPromoByCode(code)
	if err != nil {
		return nil, err
	}
	user, err := s.Repo.UpsertUser(telegramID, "", "")
	if err != nil {
		return nil, err
	}
	if err := s.Repo.SetUserPromo(user.ID, promo.ID); err != nil {
		return nil, err
	}
	return promo, nil
}
