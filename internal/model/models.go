package model

import (
	"fmt"
	"strings"
	"time"
)

// Категории товаров.
const (
	CategoryStandard = "Стандарт"
	CategoryPremium  = "Премиум"
	CategoryLux      = "Люкс"
	CategoryWow      = "WOW"
)

var Categories = []string{CategoryStandard, CategoryPremium, CategoryLux, CategoryWow}

// Способы доставки. Магазина и самовывоза нет, вариантов ровно два:
//
//	metro   — курьер отдаёт букет на станции метро, бесплатно (входит в цену букета);
//	address — курьер Яндекса по адресу; стоимость зависит от адреса, её называет
//	          менеджер вручную. Приложение доставку не считает и не хранит:
//	          сумма заказа = только букеты.
const (
	DeliveryMetro   = "metro"
	DeliveryAddress = "address"
)

var DeliveryTypes = []string{DeliveryMetro, DeliveryAddress}

// DeliveryTypeLabels — короткие подписи для админских списков, CSV и статистики.
var DeliveryTypeLabels = map[string]string{
	DeliveryMetro:   "🚇 До метро",
	DeliveryAddress: "📍 По адресу",
}

// ValidDeliveryType — способ доставки из списка (пустой не принимаем: выбор обязателен).
func ValidDeliveryType(t string) bool {
	return t == DeliveryMetro || t == DeliveryAddress
}

// Статусы заказов: new → confirmed → assembling → photo_sent → delivering → delivered / cancelled.
const (
	StatusNew        = "new"
	StatusConfirmed  = "confirmed"
	StatusAssembling = "assembling"
	StatusPhotoSent  = "photo_sent"
	StatusDelivering = "delivering"
	StatusDelivered  = "delivered"
	StatusCancelled  = "cancelled"
)

var StatusLabels = map[string]string{
	StatusNew:        "🆕 Новый",
	StatusConfirmed:  "✅ Подтверждён",
	StatusAssembling: "💐 Собираем",
	StatusPhotoSent:  "📷 Фото отправлено",
	StatusDelivering: "🚗 В пути",
	StatusDelivered:  "🌸 Доставлен",
	StatusCancelled:  "❌ Отменён",
}

var StatusOrder = []string{
	StatusNew, StatusConfirmed, StatusAssembling, StatusPhotoSent,
	StatusDelivering, StatusDelivered, StatusCancelled,
}

// ClientStatusMessages — что бот пишет клиенту при смене статуса (нет ключа = не писать).
// Каждый шаблон обязан содержать ровно один %d — номер заказа (см. ClientStatusText).
var ClientStatusMessages = map[string]string{
	StatusConfirmed:  "Заказ #%d подтверждён ✅",
	StatusAssembling: "Собираем ваш букет 💐 Заказ #%d",
	StatusDelivering: "Курьер в пути 🚗 Заказ #%d",
	StatusDelivered:  "Заказ #%d доставлен. Спасибо! 🌸",
	StatusCancelled:  "Заказ #%d отменён. Если это ошибка — напишите нам.",
}

// ClientStatusText — текст уведомления клиенту о смене статуса.
// Второе значение false, если для статуса писать не нужно.
// Номер подставляется только при наличии %d в шаблоне — иначе в сообщение
// попадал бы мусор вида "%!(EXTRA uint=4)".
func ClientStatusText(status string, orderID uint) (string, bool) {
	tmpl, ok := ClientStatusMessages[status]
	if !ok || tmpl == "" {
		return "", false
	}
	if !strings.Contains(tmpl, "%d") {
		return tmpl, true
	}
	return fmt.Sprintf(tmpl, orderID), true
}

type Product struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"not null" json:"name"`
	Description string `json:"description"`
	Category    string `gorm:"not null;index" json:"category"`
	IsHidden    bool   `gorm:"not null;default:false;index" json:"is_hidden"`
	IsHit       bool   `gorm:"not null;default:false" json:"is_hit"` // бейдж «ХИТ»
	Stock       int    `gorm:"not null;default:0" json:"stock"`      // остаток (0 = не показывать «осталось N»)
	// ArchivedAt — товар «удалён» админом (soft delete): скрыт с витрины навсегда,
	// но остаётся в БД, чтобы прошлые заказы читались (order_items → product_variants).
	ArchivedAt *time.Time `gorm:"index" json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`

	Variants []ProductVariant `gorm:"constraint:OnDelete:CASCADE" json:"variants,omitempty"`
	Images   []ProductImage   `gorm:"constraint:OnDelete:CASCADE" json:"images,omitempty"`
}

type ProductVariant struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	ProductID uint `gorm:"not null;index" json:"product_id"`
	Quantity  int  `gorm:"not null" json:"quantity"`            // кол-во цветов в букете
	Price     int  `gorm:"not null" json:"price"`               // цена в рублях
	OldPrice  int  `gorm:"not null;default:0" json:"old_price"` // цена до скидки (0 = без скидки)
	// ArchivedAt — вариант заменён при правке цен, но остаётся в БД:
	// на него ссылаются order_items прошлых заказов (FK fk_order_items_variant).
	ArchivedAt *time.Time `gorm:"index" json:"archived_at,omitempty"`
}

type ProductImage struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	ProductID uint   `gorm:"not null;index" json:"product_id"`
	URL       string `gorm:"not null" json:"url"`
}

type User struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	TelegramID int64  `gorm:"uniqueIndex" json:"telegram_id"`
	Name       string `json:"name"`
	Phone      string `json:"phone"`
	// Промокод, полученный по deep-link t.me/bot?start=CODE.
	PromoCodeID *uint      `json:"promo_code_id,omitempty"`
	PromoCode   *PromoCode `json:"promo_code,omitempty"`
}

type Order struct {
	ID         uint `gorm:"primaryKey" json:"id"`
	UserID     uint `gorm:"not null;index;index:idx_orders_user_status" json:"user_id"`
	TotalPrice int  `gorm:"not null" json:"total_price"` // только букеты: стоимости доставки в системе нет
	// DeliveryType — metro (бесплатно, в цене букета) или address (курьер Яндекса,
	// цену называет менеджер вручную). См. константы Delivery*.
	DeliveryType    string `gorm:"not null;default:address;index" json:"delivery_type"`
	MetroStation    string `json:"metro_station"`                    // заполнено при DeliveryMetro
	DeliveryAddress string `gorm:"not null" json:"delivery_address"` // заполнен при DeliveryAddress
	DeliveryDate    string `gorm:"not null;index:idx_orders_status_ddate" json:"delivery_date"`
	DeliveryTime    string `gorm:"not null" json:"delivery_time"` // пусто = время не выбрано
	// DeliveryAgreedAt/By — админ созвонился с клиентом и согласовал курьера
	// (только для DeliveryAddress). Пока пусто — на карточке висит ⚠️.
	DeliveryAgreedAt *time.Time `json:"delivery_agreed_at,omitempty"`
	DeliveryAgreedBy int64      `gorm:"not null;default:0" json:"-"` // telegram_id админа
	PromoCodeID      *uint      `json:"promo_code_id,omitempty"`
	Comment          string     `json:"comment"`
	CardText         string     `json:"card_text"`     // текст открытки (до 300 символов)
	IsAnonymous      bool       `json:"is_anonymous"`  // анонимная доставка
	CancelReason     string     `json:"cancel_reason"` // причина отмены (обязательна при отмене админом)
	Status           string     `gorm:"not null;default:new;index;index:idx_orders_status_ddate,priority:1;index:idx_orders_user_status,priority:2" json:"status"`
	CreatedAt        time.Time  `json:"created_at"`

	User      User        `json:"user"`
	PromoCode *PromoCode  `json:"promo_code,omitempty"`
	Items     []OrderItem `gorm:"constraint:OnDelete:CASCADE" json:"items"`
}

// IsMetroDelivery — доставка до станции метро. Всё остальное (включая заказы,
// оформленные до появления выбора, — у них тип пустой) считаем курьерским:
// лучше лишний раз показать «менеджер свяжется», чем пообещать бесплатное метро.
func (o *Order) IsMetroDelivery() bool { return o.DeliveryType == DeliveryMetro }

// DeliveryText — что видит клиент: подтверждение, история, уведомления бота.
// Одна формулировка на все каналы, чтобы обещание нигде не расходилось.
func (o *Order) DeliveryText() string {
	if o.IsMetroDelivery() {
		return fmt.Sprintf("Доставка до метро %s — бесплатно", orDashText(o.MetroStation))
	}
	return "Доставка по адресу — менеджер свяжется и назовёт стоимость курьера"
}

// AdminDeliveryLine — первая строка карточки заказа в админ-чате:
// человеку, который собирает и везёт букет, это нужно раньше всего остального.
func (o *Order) AdminDeliveryLine() string {
	if o.IsMetroDelivery() {
		return "🚇 Метро: " + orDashText(o.MetroStation)
	}
	return "📍 По адресу: " + orDashText(o.DeliveryAddress)
}

// NeedsDeliveryApproval — заказ по адресу, где курьера ещё не согласовали с клиентом.
// У завершённых заказов маркер не показываем: согласовывать уже нечего.
func (o *Order) NeedsDeliveryApproval() bool {
	if o.IsMetroDelivery() || o.DeliveryAgreedAt != nil {
		return false
	}
	return o.Status != StatusDelivered && o.Status != StatusCancelled
}

// DeliveryTypeLabel — подпись способа доставки с запасом на старые заказы
// без проставленного типа.
func DeliveryTypeLabel(t string) string {
	if t == DeliveryMetro {
		return DeliveryTypeLabels[DeliveryMetro]
	}
	return DeliveryTypeLabels[DeliveryAddress]
}

func orDashText(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

type OrderItem struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	OrderID   uint `gorm:"not null;index" json:"order_id"`
	VariantID uint `gorm:"not null" json:"variant_id"`
	Quantity  int  `gorm:"not null" json:"quantity"`
	Price     int  `gorm:"not null" json:"price"` // цена за единицу на момент заказа

	Variant ProductVariant `json:"variant"`
	// Название товара фиксируем на момент заказа, чтобы заказ читался даже после удаления товара.
	ProductName string `json:"product_name"`
}

// OrderStatusLog — журнал смен статусов для разбора спорных ситуаций.
type OrderStatusLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	OrderID    uint      `gorm:"not null;index" json:"order_id"`
	FromStatus string    `gorm:"not null" json:"from_status"`
	ToStatus   string    `gorm:"not null" json:"to_status"`
	AdminID    int64     `gorm:"not null" json:"admin_id"` // telegram_id админа (0 = система)
	CreatedAt  time.Time `json:"created_at"`
}

// NextStatus — следующий шаг конвейера (пустая строка = терминальный статус).
func NextStatus(s string) string {
	for i, st := range StatusOrder {
		if st == s && i+1 < len(StatusOrder) {
			next := StatusOrder[i+1]
			if next == StatusCancelled {
				return ""
			}
			return next
		}
	}
	return ""
}

// AllowedTransition — конечный автомат: только следующий шаг либо отмена
// из нетерминального статуса (никаких прыжков new → delivered).
func AllowedTransition(from, to string) bool {
	if to == StatusCancelled {
		return from != StatusDelivered && from != StatusCancelled
	}
	return NextStatus(from) == to
}

// Upload — фото товара, сохранённое в БД (для хостинга без постоянного диска).
type Upload struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Ext       string    `gorm:"not null" json:"ext"`       // .jpg / .webp
	MimeType  string    `gorm:"not null" json:"mime_type"` // image/jpeg
	Data      []byte    `gorm:"type:bytea;not null" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// FreshToday — «Сегодня на базе»: что флорист закупил утром.
// Актуальна запись за сегодняшнюю дату; /fresh в боте перезаписывает её.
type FreshToday struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Date      string    `gorm:"uniqueIndex;not null" json:"date"` // YYYY-MM-DD
	Items     string    `gorm:"not null" json:"items"`            // «пионы, ранункулюсы, эустома»
	CreatedAt time.Time `json:"created_at"`
}

type PromoCode struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	Code            string `gorm:"uniqueIndex;not null" json:"code"`
	DiscountPercent int    `gorm:"not null" json:"discount_percent"`
	Uses            int    `gorm:"not null;default:0" json:"uses"`
}
