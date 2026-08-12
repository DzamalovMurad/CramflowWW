// Package service — бизнес-логика: расчёт сумм заказа и промокоды.
package service

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

// Доставка ежедневно с 9:00 до 21:00: в течение часа или к точному времени («к 15:30»).
var DeliveryOptions = []string{"в течение часа"}

var deliveryAtRe = regexp.MustCompile(`^к ([0-2]\d):([0-5]\d)$`)

// validDeliveryTime принимает готовый вариант или «к HH:MM» в окне 9:00–21:00.
// Пустую строку проверяет вызывающий: время — пожелание, а не обязательное поле
// (до метро клиент может его не указывать, по адресу его согласует менеджер).
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
	Items []OrderItemInput `json:"items"`
	Name  string           `json:"name"`
	Phone string           `json:"phone"`
	// DeliveryType — metro | address (см. model.Delivery*). Обязателен.
	// От него зависит, что требуем дальше: станцию метро или адрес.
	DeliveryType    string `json:"delivery_type"`
	MetroStation    string `json:"metro_station"`
	DeliveryAddress string `json:"delivery_address"`
	DeliveryDate    string `json:"delivery_date"`
	DeliveryTime    string `json:"delivery_time"`
	Comment         string `json:"comment"`
	CardText        string `json:"card_text"`    // текст открытки, до 300 символов
	IsAnonymous     bool   `json:"is_anonymous"` // анонимная доставка
	PromoCode       string `json:"promo_code"`
	// TelegramID заполняется хендлером из initData, не клиентом.
	TelegramID int64 `json:"-"`
}

type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// validateOrderInput чистит поля заказа и проверяет их. Возвращает нормализованный
// вход: станция приводится к каноничному написанию, а лишнее для выбранного способа
// доставки поле обнуляется — в заказе не должно остаться адреса при доставке
// до метро и наоборот.
func validateOrderInput(in OrderInput) (OrderInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	in.DeliveryType = strings.TrimSpace(in.DeliveryType)
	in.MetroStation = strings.TrimSpace(in.MetroStation)
	in.DeliveryAddress = strings.TrimSpace(in.DeliveryAddress)
	in.DeliveryDate = strings.TrimSpace(in.DeliveryDate)
	in.DeliveryTime = strings.TrimSpace(in.DeliveryTime)
	in.Comment = strings.TrimSpace(in.Comment)
	in.CardText = strings.TrimSpace(in.CardText)
	if len([]rune(in.CardText)) > 300 {
		return in, invalid("текст открытки — не более 300 символов")
	}

	if len(in.Items) == 0 {
		return in, invalid("корзина пуста")
	}
	if in.Name == "" {
		return in, invalid("укажите имя")
	}
	if len(in.Phone) < 6 {
		return in, invalid("укажите корректный телефон")
	}
	// Способ доставки определяет, какое поле обязательно дальше.
	if !model.ValidDeliveryType(in.DeliveryType) {
		return in, invalid("выберите способ доставки")
	}
	if in.DeliveryType == model.DeliveryMetro {
		station, ok := model.NormalizeMetroStation(in.MetroStation)
		if !ok {
			return in, invalid("выберите станцию метро из списка")
		}
		in.MetroStation = station
		in.DeliveryAddress = ""
	} else {
		if in.DeliveryAddress == "" {
			return in, invalid("укажите адрес доставки")
		}
		in.MetroStation = ""
	}
	if in.DeliveryDate == "" {
		return in, invalid("укажите дату доставки")
	}
	// Время необязательно: до метро это пожелание клиента, по адресу его
	// согласует менеджер. Но если указано — только в окне работы курьеров.
	if in.DeliveryTime != "" && !validDeliveryTime(in.DeliveryTime) {
		return in, invalid("выберите время доставки (с 9:00 до 21:00)")
	}
	return in, nil
}

// CreateOrder валидирует вход, считает сумму по ценам из БД, применяет промокод
// (переданный явно или сохранённый у пользователя по deep-link) и сохраняет заказ.
func (s *Service) CreateOrder(in OrderInput) (*model.Order, error) {
	in, err := validateOrderInput(in)
	if err != nil {
		return nil, err
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

	// TotalPrice — только букеты: стоимости доставки в системе нет ни в каком виде.
	order := &model.Order{
		UserID:          user.ID,
		TotalPrice:      total,
		DeliveryType:    in.DeliveryType,
		MetroStation:    in.MetroStation,
		DeliveryAddress: in.DeliveryAddress,
		DeliveryDate:    in.DeliveryDate,
		DeliveryTime:    in.DeliveryTime,
		PromoCodeID:     promoID,
		Comment:         in.Comment,
		CardText:        in.CardText,
		IsAnonymous:     in.IsAnonymous,
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

// AgreeDelivery — админ созвонился с клиентом, назвал цену курьера и согласовал время.
// Снимает маркер «⚠️ Согласовать доставку» с карточки заказа по адресу.
func (s *Service) AgreeDelivery(orderID uint, adminID int64) (*model.Order, error) {
	order, err := s.Repo.GetOrder(orderID)
	if err != nil {
		return nil, err
	}
	if order.IsMetroDelivery() {
		return nil, invalid("заказ с доставкой до метро — согласовывать нечего")
	}
	if order.DeliveryAgreedAt != nil {
		return nil, invalid("доставка уже согласована")
	}
	if err := s.Repo.MarkDeliveryAgreed(orderID, adminID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, invalid("доставка уже согласована — обновите карточку")
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
	user, err := s.Repo.UpsertUser(telegramID, "", "")
	if err != nil {
		return nil, err
	}
	if err := s.Repo.SetUserPromo(user.ID, promo.ID); err != nil {
		return nil, err
	}
	return promo, nil
}
