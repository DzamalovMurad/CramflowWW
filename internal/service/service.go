// Package service — бизнес-логика: слоты доставки, расчёт сумм заказа и промокоды.
package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

type Service struct {
	Repo *repository.Repository
	// SlotCapacity — максимум заказов в один слот доставки (env SLOT_CAPACITY).
	SlotCapacity int
	// NotifyNewOrder вызывается после сохранения заказа (бот шлёт сообщение админу).
	NotifyNewOrder func(o *model.Order)
}

func New(repo *repository.Repository) *Service {
	return &Service{Repo: repo, SlotCapacity: DefaultSlotCapacity}
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
	DeliveryDate    string           `json:"delivery_date"` // YYYY-MM-DD
	DeliveryTime    string           `json:"delivery_time"` // слот «10:00-12:00» … «20:00-22:00»
	Comment         string           `json:"comment"`
	CardText        string           `json:"card_text"`    // текст открытки, до 200 символов
	IsAnonymous     bool             `json:"is_anonymous"` // анонимная доставка

	// Подарочный флоу «заказываю не себе».
	RecipientName      string `json:"recipient_name"`
	RecipientPhone     string `json:"recipient_phone"`
	AddressByRecipient bool   `json:"address_by_recipient"` // адрес уточнит курьер у получателя

	PromoCode string `json:"promo_code"`
	// IdempotencyKey — uuid от клиента: повтор запроса возвращает тот же заказ.
	IdempotencyKey string `json:"idempotency_key"`
	// TelegramID заполняется хендлером из initData, не клиентом.
	TelegramID int64 `json:"-"`
}

type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// pricedItem — позиция заказа с товаром: цены из БД + данные для applies_to промокода.
type pricedItem struct {
	item    model.OrderItem
	product *model.Product
}

// buildItems собирает позиции по ценам из БД — клиентским ценам не доверяем.
func (s *Service) buildItems(inputs []OrderItemInput) ([]pricedItem, int, error) {
	if len(inputs) == 0 {
		return nil, 0, invalid("корзина пуста")
	}
	total := 0
	priced := make([]pricedItem, 0, len(inputs))
	for _, it := range inputs {
		if it.Quantity < 1 || it.Quantity > 99 {
			return nil, 0, invalid("некорректное количество")
		}
		variant, err := s.Repo.GetVariant(it.VariantID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, 0, invalid("товар из корзины больше недоступен")
			}
			return nil, 0, err
		}
		product, err := s.Repo.GetProduct(variant.ProductID)
		if err != nil || product.IsHidden || product.ArchivedAt != nil {
			return nil, 0, invalid("товар из корзины больше недоступен")
		}
		total += variant.Price * it.Quantity
		priced = append(priced, pricedItem{
			item: model.OrderItem{
				VariantID:   variant.ID,
				Quantity:    it.Quantity,
				Price:       variant.Price,
				ProductName: product.Name,
			},
			product: product,
		})
	}
	return priced, total, nil
}

// CreateOrder валидирует вход (слот, получатель, промокод), считает сумму по
// ценам из БД и сохраняет заказ одной транзакцией: занятость слота и лимиты
// промокода проверяются под блокировкой — гонки не создают ни перегруза слота,
// ни лишних применений кода.
func (s *Service) CreateOrder(in OrderInput) (*model.Order, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	in.DeliveryAddress = strings.TrimSpace(in.DeliveryAddress)
	in.DeliveryDate = strings.TrimSpace(in.DeliveryDate)
	in.Comment = strings.TrimSpace(in.Comment)
	in.CardText = strings.TrimSpace(in.CardText)
	in.RecipientName = strings.TrimSpace(in.RecipientName)
	in.RecipientPhone = strings.TrimSpace(in.RecipientPhone)
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)

	// Идемпотентность: повтор запроса с тем же ключом возвращает созданный заказ.
	if len(in.IdempotencyKey) > 64 {
		return nil, invalid("некорректный ключ запроса")
	}
	if in.IdempotencyKey != "" {
		if existing, err := s.Repo.GetOrderByIdempotencyKey(in.IdempotencyKey); err == nil {
			if existing.User.TelegramID != in.TelegramID {
				return nil, invalid("некорректный ключ запроса")
			}
			return existing, nil
		}
	}

	if in.Name == "" {
		return nil, invalid("укажите имя")
	}
	if len(in.Phone) < 6 {
		return nil, invalid("укажите корректный телефон")
	}
	if len([]rune(in.CardText)) > 200 {
		return nil, invalid("текст открытки — не более 200 символов")
	}

	// Получатель: если заказ «не себе», нужны его имя и телефон.
	forRecipient := in.RecipientName != "" || in.RecipientPhone != "" || in.AddressByRecipient
	if forRecipient {
		if in.RecipientName == "" {
			return nil, invalid("укажите имя получателя")
		}
		if len(in.RecipientPhone) < 6 {
			return nil, invalid("укажите телефон получателя")
		}
	}
	// Адрес обязателен, если курьер не уточняет его у получателя.
	if in.DeliveryAddress == "" && !in.AddressByRecipient {
		return nil, invalid("укажите адрес доставки")
	}

	if err := s.validateSlot(in.DeliveryDate, in.DeliveryTime); err != nil {
		return nil, err
	}

	priced, total, err := s.buildItems(in.Items)
	if err != nil {
		return nil, err
	}

	user, err := s.Repo.UpsertUser(in.TelegramID, in.Name, in.Phone)
	if err != nil {
		return nil, err
	}

	// Промокод: явный из checkout приоритетнее сохранённого по deep-link.
	// Один промокод на заказ. Ошибка явного кода блокирует заказ, протухший
	// deep-link-код просто не применяется.
	var promo *model.PromoCode
	discount := 0
	if code := strings.TrimSpace(in.PromoCode); code != "" {
		promo, discount, err = s.ValidatePromo(code, in.TelegramID, in.Items)
		if err != nil {
			return nil, err
		}
	} else if user.PromoCodeID != nil {
		if saved, err := s.Repo.GetPromoByID(*user.PromoCodeID); err == nil {
			if d, err := s.checkPromoRules(saved, user, priced, total); err == nil {
				promo, discount = saved, d
			}
		}
	}

	items := make([]model.OrderItem, 0, len(priced))
	for _, pi := range priced {
		items = append(items, pi.item)
	}

	var promoID *uint
	if promo != nil {
		promoID = &promo.ID
	}
	var idemKey *string
	if in.IdempotencyKey != "" {
		idemKey = &in.IdempotencyKey
	}

	order := &model.Order{
		UserID:             user.ID,
		TotalPrice:         total - discount,
		DiscountAmount:     discount,
		DeliveryAddress:    in.DeliveryAddress,
		DeliveryDate:       in.DeliveryDate,
		DeliveryTime:       in.DeliveryTime,
		PromoCodeID:        promoID,
		Comment:            in.Comment,
		RecipientName:      in.RecipientName,
		RecipientPhone:     in.RecipientPhone,
		AddressByRecipient: in.AddressByRecipient,
		CardText:           in.CardText,
		IsAnonymous:        in.IsAnonymous,
		IdempotencyKey:     idemKey,
		Status:             model.StatusNew,
		Items:              items,
	}
	if err := s.Repo.CreateOrderChecked(order, s.SlotCapacity, promo); err != nil {
		switch {
		case errors.Is(err, repository.ErrSlotFull):
			return nil, invalid("этот слот доставки уже занят — выберите другой")
		case errors.Is(err, repository.ErrPromoExhausted):
			return nil, invalid("Лимит использований промокода исчерпан")
		case errors.Is(err, repository.ErrPromoAlreadyUsed):
			return nil, invalid("Вы уже использовали этот промокод")
		}
		// Гонка идемпотентности: параллельный повтор успел вставить заказ первым —
		// уникальный индекс по ключу сработал, отдаём существующий заказ.
		if in.IdempotencyKey != "" {
			if existing, lookupErr := s.Repo.GetOrderByIdempotencyKey(in.IdempotencyKey); lookupErr == nil &&
				existing.User.TelegramID == in.TelegramID {
				return existing, nil
			}
		}
		return nil, err
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
	return s.Repo.GetOrder(orderID)
}

// ApplyDeepLinkPromo сохраняет промокод у пользователя (deep-link t.me/bot?start=CODE).
func (s *Service) ApplyDeepLinkPromo(telegramID int64, code string) (*model.PromoCode, error) {
	promo, err := s.Repo.GetPromoByCode(code)
	if err != nil {
		return nil, err
	}
	if !promo.DisplayUsable(time.Now()) {
		return nil, errors.New("промокод недоступен")
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

// RepeatOrderItem — позиция «повторить заказ»: актуальный вариант либо причина недоступности.
type RepeatOrderItem struct {
	Available    bool   `json:"available"`
	Reason       string `json:"reason,omitempty"`
	ProductID    uint   `json:"product_id,omitempty"`
	ProductName  string `json:"product_name"`
	VariantID    uint   `json:"variant_id,omitempty"`
	FlowersCount int    `json:"flowers_count,omitempty"`
	Price        int    `json:"price,omitempty"`
	Image        string `json:"image,omitempty"`
	Qty          int    `json:"qty"`
}

// RepeatOrder собирает позиции прошлого заказа по актуальному каталогу:
// цены и варианты берутся текущие; если вариант архивирован правкой цен,
// подбирается живой с тем же количеством цветов. Недоступное помечается причиной.
func (s *Service) RepeatOrder(o *model.Order) []RepeatOrderItem {
	out := make([]RepeatOrderItem, 0, len(o.Items))
	for _, it := range o.Items {
		ri := RepeatOrderItem{ProductName: it.ProductName, Qty: it.Quantity}
		variant, err := s.Repo.GetVariantAny(it.VariantID)
		if err != nil {
			ri.Reason = "товар больше не продаётся"
			out = append(out, ri)
			continue
		}
		if variant.ArchivedAt != nil {
			variant, err = s.Repo.FindLiveVariant(variant.ProductID, variant.Quantity)
			if err != nil {
				ri.Reason = "этот размер букета больше недоступен"
				out = append(out, ri)
				continue
			}
		}
		p, err := s.Repo.GetProduct(variant.ProductID)
		if err != nil || p.IsHidden || p.ArchivedAt != nil {
			ri.Reason = "товар больше не продаётся"
			out = append(out, ri)
			continue
		}
		ri.Available = true
		ri.ProductID = p.ID
		ri.ProductName = p.Name
		ri.VariantID = variant.ID
		ri.FlowersCount = variant.Quantity
		ri.Price = variant.Price
		if len(p.Images) > 0 {
			ri.Image = p.Images[0].URL
		}
		out = append(out, ri)
	}
	return out
}
