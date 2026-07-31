// Package model — доменные сущности и правила, не зависящие от БД и транспорта.
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

// IsCategory — категория из известного списка.
func IsCategory(s string) bool {
	for _, c := range Categories {
		if c == s {
			return true
		}
	}
	return false
}

// Статусы заказа: new → confirmed → assembling → delivering → delivered,
// отмена возможна из любого нетерминального статуса.
//
// Отправка фото букета клиенту — отдельное действие, а не шаг конвейера:
// раньше статус photo_sent приходилось «проходить» даже без фото, из-за чего
// клиент получал уведомление о несуществующем событии, а админ делал лишний тап.
const (
	StatusNew        = "new"
	StatusConfirmed  = "confirmed"
	StatusAssembling = "assembling"
	StatusDelivering = "delivering"
	StatusDelivered  = "delivered"
	StatusCancelled  = "cancelled"
)

var StatusLabels = map[string]string{
	StatusNew:        "🆕 Новый",
	StatusConfirmed:  "✅ Подтверждён",
	StatusAssembling: "💐 Собираем",
	StatusDelivering: "🚗 В пути",
	StatusDelivered:  "🌸 Доставлен",
	StatusCancelled:  "❌ Отменён",
}

// pipeline — линейный конвейер выполнения заказа.
var pipeline = []string{StatusNew, StatusConfirmed, StatusAssembling, StatusDelivering, StatusDelivered}

// StatusOrder — порядок статусов для фильтров админки (конвейер + отмена).
var StatusOrder = append(append([]string{}, pipeline...), StatusCancelled)

// ActiveStatuses — заказы «в работе»: ещё не доставлены и не отменены.
var ActiveStatuses = []string{StatusNew, StatusConfirmed, StatusAssembling, StatusDelivering}

// IsTerminal — из статуса больше некуда двигаться.
func IsTerminal(s string) bool { return s == StatusDelivered || s == StatusCancelled }

// ClientStatusMessages — что бот пишет клиенту при смене статуса
// (нет ключа = клиенту не пишем). Шаблон может содержать один %d — номер заказа.
var ClientStatusMessages = map[string]string{
	StatusConfirmed:  "Заказ #%d подтверждён ✅ Уже готовим его к сборке.",
	StatusAssembling: "Собираем ваш букет 💐 Заказ #%d",
	StatusDelivering: "Курьер в пути 🚗 Заказ #%d",
	StatusDelivered:  "Заказ #%d доставлен. Спасибо, что выбрали нас! 🌸",
	StatusCancelled:  "Заказ #%d отменён. Если это ошибка — напишите нам.",
}

// ClientStatusText — текст уведомления клиенту о смене статуса.
// Второе значение false, если для статуса писать не нужно.
// Номер подставляется только при наличии %d — иначе в сообщение попадал бы
// мусор вида "%!(EXTRA uint=4)".
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

// NextStatus — следующий шаг конвейера (пустая строка = дальше некуда).
func NextStatus(s string) string {
	for i, st := range pipeline {
		if st == s && i+1 < len(pipeline) {
			return pipeline[i+1]
		}
	}
	return ""
}

// AllowedTransition — конечный автомат: только следующий шаг конвейера
// либо отмена из нетерминального статуса (никаких прыжков new → delivered).
func AllowedTransition(from, to string) bool {
	if to == StatusCancelled {
		return !IsTerminal(from)
	}
	return NextStatus(from) == to
}

// ─── Сущности ──────────────────────────────────────────────────────────────

type Product struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	IsHidden    bool   `json:"is_hidden"`
	IsHit       bool   `json:"is_hit"` // бейдж «ХИТ»
	// Stock nil = количество не ограничено (магазин не ведёт учёт по этой позиции).
	// Число — реальный остаток: уменьшается при заказе, возвращается при отмене,
	// 0 означает «закончилось» и товар нельзя заказать.
	Stock *int `json:"stock"`
	// ArchivedAt — товар «удалён» админом (soft delete): скрыт с витрины навсегда,
	// но остаётся в БД, чтобы прошлые заказы читались (order_items → product_variants).
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`

	Variants []ProductVariant `json:"variants,omitempty"`
	Images   []ProductImage   `json:"images,omitempty"`
}

// Available — товар можно показывать на витрине.
func (p *Product) Available() bool { return !p.IsHidden && p.ArchivedAt == nil }

// InStock — товар можно заказать: либо учёт не ведётся, либо остаток положительный.
func (p *Product) InStock() bool { return p.Stock == nil || *p.Stock > 0 }

type ProductVariant struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	ProductID uint `json:"product_id"`
	Quantity  int  `json:"quantity"`  // кол-во цветов в букете
	Price     int  `json:"price"`     // цена в целых рублях
	OldPrice  int  `json:"old_price"` // цена до скидки (0 = без скидки)
	// ArchivedAt — вариант заменён при правке цен, но остаётся в БД:
	// на него ссылаются order_items прошлых заказов.
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
}

type ProductImage struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	ProductID uint   `json:"product_id"`
	URL       string `json:"url"`
}

type User struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	TelegramID int64     `json:"telegram_id"`
	Name       string    `json:"name"`
	Phone      string    `json:"phone"`
	CreatedAt  time.Time `json:"created_at"`
	// Промокод, полученный по deep-link ?start=CODE.
	PromoCodeID *uint      `json:"promo_code_id,omitempty"`
	PromoCode   *PromoCode `json:"promo_code,omitempty"`
}

type Order struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	UserID uint `json:"user_id"`
	// Деньги — только целые рубли, считаются на сервере: total = subtotal − discount.
	SubtotalPrice   int    `json:"subtotal_price"`
	DiscountAmount  int    `json:"discount_amount"`
	TotalPrice      int    `json:"total_price"`
	DeliveryAddress string `json:"delivery_address"`
	DeliveryDate    string `json:"delivery_date"` // YYYY-MM-DD в часовом поясе магазина
	DeliveryTime    string `json:"delivery_time"`
	// Получатель, если это не сам заказчик (подарок). Пусто = получатель = заказчик.
	RecipientName  string    `json:"recipient_name"`
	RecipientPhone string    `json:"recipient_phone"`
	PromoCodeID    *uint     `json:"promo_code_id,omitempty"`
	Comment        string    `json:"comment"`
	CardText       string    `json:"card_text"`    // текст открытки
	IsAnonymous    bool      `json:"is_anonymous"` // анонимная доставка
	CancelReason   string    `json:"cancel_reason"`
	Status         string    `json:"status"`
	IdempotencyKey *string   `json:"-"` // ключ повторной отправки формы
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	User      User        `json:"user"`
	PromoCode *PromoCode  `json:"promo_code,omitempty"`
	Items     []OrderItem `json:"items"`
}

type OrderItem struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	OrderID   uint `json:"order_id"`
	VariantID uint `json:"variant_id"`
	Quantity  int  `json:"quantity"`
	Price     int  `json:"price"` // цена за единицу на момент заказа

	Variant ProductVariant `json:"variant"`
	// Название товара фиксируем на момент заказа, чтобы заказ читался
	// даже после удаления товара.
	ProductName string `json:"product_name"`
}

// OrderStatusLog — журнал смен статусов для разбора спорных ситуаций.
type OrderStatusLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	OrderID    uint      `json:"order_id"`
	FromStatus string    `json:"from_status"`
	ToStatus   string    `json:"to_status"`
	AdminID    int64     `json:"admin_id"` // telegram_id админа (0 = система)
	CreatedAt  time.Time `json:"created_at"`
}

// Upload — фото товара, сохранённое в БД (хостинг без постоянного диска).
type Upload struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Ext       string    `json:"ext"`       // .jpg
	MimeType  string    `json:"mime_type"` // image/jpeg
	Data      []byte    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// FreshToday — «Сегодня на базе»: что флорист закупил утром.
type FreshToday struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Date      string    `json:"date"` // YYYY-MM-DD
	Items     string    `json:"items"`
	CreatedAt time.Time `json:"created_at"`
}

type PromoCode struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	Code            string     `json:"code"`
	DiscountPercent int        `json:"discount_percent"`
	Uses            int        `json:"uses"`
	MaxUses         int        `json:"max_uses"`       // 0 = без ограничения
	PerUserLimit    int        `json:"per_user_limit"` // 0 = без ограничения
	IsActive        bool       `json:"is_active"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// Usable — код в принципе действует (без учёта персональных лимитов клиента).
func (p *PromoCode) Usable(now time.Time) bool {
	if !p.IsActive {
		return false
	}
	if p.ExpiresAt != nil && now.After(*p.ExpiresAt) {
		return false
	}
	if p.MaxUses > 0 && p.Uses >= p.MaxUses {
		return false
	}
	return true
}

// PromoRedemption — факт применения промокода конкретным клиентом в заказе.
type PromoRedemption struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	PromoCodeID uint      `json:"promo_code_id"`
	UserID      uint      `json:"user_id"`
	OrderID     uint      `json:"order_id"`
	CreatedAt   time.Time `json:"created_at"`
}

// ─── Деньги ────────────────────────────────────────────────────────────────

// ApplyDiscount считает скидку и итог в целых рублях.
// Скидка округляется вниз — в пользу магазина и без копеечных расхождений.
func ApplyDiscount(subtotal, percent int) (discount, total int) {
	if subtotal <= 0 || percent <= 0 {
		return 0, max(subtotal, 0)
	}
	if percent > 100 {
		percent = 100
	}
	discount = subtotal * percent / 100
	return discount, subtotal - discount
}
