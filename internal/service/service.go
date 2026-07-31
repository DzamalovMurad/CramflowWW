// Package service — бизнес-логика: расчёт сумм заказа и промокоды.
package service

import (
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

// Доставка ежедневно с 9:00 до 21:00: в течение часа или к точному времени («к 15:30»).
var DeliveryOptions = []string{"в течение часа"}

var deliveryAtRe = regexp.MustCompile(`^к ([0-2]\d):([0-5]\d)$`)

// validDeliveryTime принимает готовый вариант или «к HH:MM» в окне 9:00–21:00.
func validDeliveryTime(s string) bool {
	for _, opt := range DeliveryOptions {
		if s == opt {
			return true
		}
	}
	m := deliveryAtRe.FindStringSubmatch(s)
	if m == nil {
		return false
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	t := h*60 + min
	return t >= 9*60 && t <= 21*60
}

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
	CardText        string           `json:"card_text"`    // текст открытки, до 300 символов
	IsAnonymous     bool             `json:"is_anonymous"` // анонимная доставка
	PromoCode       string           `json:"promo_code"`
	// TelegramID и Source заполняются хендлером из initData, не клиентом.
	TelegramID int64  `json:"-"`
	Source     string `json:"-"` // источник запуска Mini App (startapp-параметр)
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
	in.CardText = strings.TrimSpace(in.CardText)
	if len([]rune(in.CardText)) > 300 {
		return nil, invalid("текст открытки — не более 300 символов")
	}

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
	if !validDeliveryTime(in.DeliveryTime) {
		return nil, invalid("выберите время доставки (с 9:00 до 21:00)")
	}

	user, err := s.Repo.UpsertUser(in.TelegramID, in.Name, in.Phone)
	if err != nil {
		return nil, err
	}
	// First-touch источник: если клиент впервые попал к нам через checkout
	// (не открывал профиль/главную с трекингом), фиксируем источник здесь же.
	if in.Source != "" {
		_ = s.Repo.SetUserAcquisitionSource(user.ID, in.Source)
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
		CardText:        in.CardText,
		IsAnonymous:     in.IsAnonymous,
		Status:          model.StatusNew,
		Source:          in.Source,
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

// TransitionOrder — смена статуса через конечный автомат: только следующий шаг
// либо отмена (с обязательной причиной) из нетерминального статуса.
// Возвращает обновлённый заказ.
func (s *Service) TransitionOrder(orderID uint, to string, adminID int64, cancelReason string) (*model.Order, error) {
	order, err := s.Repo.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	if !model.AllowedTransition(order.Status, to) {
		return nil, invalid("переход %s → %s недопустим", model.StatusLabels[order.Status], model.StatusLabels[to])
	}
	if to == model.StatusCancelled && strings.TrimSpace(cancelReason) == "" {
		return nil, invalid("укажите причину отмены")
	}
	if err := s.Repo.ChangeOrderStatus(orderID, order.Status, to, adminID, strings.TrimSpace(cancelReason)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, invalid("статус заказа уже изменился — обновите карточку")
		}
		return nil, err
	}
	// Доставлен → через 2 часа спросим про букет. План живёт в БД (notifications),
	// так что перезапуск сервиса ничего не теряет; дубликаты гасит уникальный индекс.
	if to == model.StatusDelivered {
		if err := s.Repo.ScheduleNotification(orderID, model.NotificationFeedback, time.Now().Add(model.FeedbackDelay)); err != nil {
			log.Printf("планирование отзыва по заказу #%d: %v", orderID, err)
		}
	}
	return s.Repo.GetOrder(orderID)
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
