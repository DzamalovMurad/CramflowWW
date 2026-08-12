// Package model — доменные сущности и правила, не зависящие от БД и транспорта.
package model

import (
	"fmt"
	"strconv"
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
	// FreshUntil — до какого момента товар помечен как свежая поставка.
	// Срок, а не флаг: бейдж «СВЕЖЕЕ» гаснет сам, без уборки по расписанию.
	FreshUntil *time.Time `json:"-"`
	// DailyPickOn — календарный день магазина (YYYY-MM-DD), когда этот букет
	// был выбран «букетом дня». Уникальный индекс не даст назначить второй.
	DailyPickOn *string `json:"-"`
	// ArchivedAt — товар «удалён» админом (soft delete): скрыт с витрины навсегда,
	// но остаётся в БД, чтобы прошлые заказы читались (order_items → product_variants).
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`

	Variants []ProductVariant `json:"variants,omitempty"`
	Images   []ProductImage   `json:"images,omitempty"`

	// Признаки бейджей для витрины: считаются от времени магазина в хендлере
	// (StampBadges) и в БД не хранятся — клиенту незачем знать сроки и даты,
	// ему нужен ответ «показывать или нет».
	IsFresh     bool `gorm:"-" json:"is_fresh"`
	IsDailyPick bool `gorm:"-" json:"is_daily_pick"`
}

// Fresh — товар помечен как свежая поставка и метка ещё не протухла.
func (p *Product) Fresh(now time.Time) bool {
	return p.FreshUntil != nil && now.Before(*p.FreshUntil)
}

// DailyPick — товар назначен букетом дня именно на сегодня.
// today — календарный день магазина (Config.Today), не дата контейнера.
func (p *Product) DailyPick(today string) bool {
	return p.DailyPickOn != nil && *p.DailyPickOn == today
}

// StampBadges проставляет витринные признаки перед отдачей клиенту.
func (p *Product) StampBadges(now time.Time, today string) {
	p.IsFresh = p.Fresh(now)
	p.IsDailyPick = p.DailyPick(today)
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
	RecipientName  string `json:"recipient_name"`
	RecipientPhone string `json:"recipient_phone"`
	PromoCodeID    *uint  `json:"promo_code_id,omitempty"`
	// Код, применённый в этом заказе, зафиксированный на момент оформления:
	// сам промокод могут выключить, переписать или удалить, а заказ должен
	// объяснять свою скидку и через год.
	AppliedPromoCode string    `json:"applied_promo_code,omitempty"`
	Comment          string    `json:"comment"`
	CardText         string    `json:"card_text"`    // текст открытки
	IsAnonymous      bool      `json:"is_anonymous"` // анонимная доставка
	CancelReason     string    `json:"cancel_reason"`
	Status           string    `json:"status"`
	IdempotencyKey   *string   `json:"-"` // ключ повторной отправки формы
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`

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

// Типы скидки промокода.
const (
	DiscountTypePercent = "percent" // процент от суммы заказа
	DiscountTypeFixed   = "fixed"   // фиксированная сумма в рублях
)

// Границы правила скидки. Те же значения проверяет CHECK в миграции 0002:
// база — последний рубеж, приложение — понятное сообщение об ошибке.
const (
	MaxDiscountPercent = 90
	MaxDiscountFixed   = 1_000_000
)

type PromoCode struct {
	ID   uint   `gorm:"primaryKey" json:"id"`
	Code string `json:"code"`
	// Правило скидки: DiscountValue читается в зависимости от DiscountType —
	// проценты для percent, рубли для fixed.
	DiscountType  string `json:"discount_type"`
	DiscountValue int    `json:"discount_value"`
	// Минимальная сумма заказа до скидки; 0 = без ограничения.
	MinOrderAmount int        `json:"min_order_amount"`
	Uses           int        `json:"uses"`
	MaxUses        int        `json:"max_uses"`       // 0 = без ограничения
	PerUserLimit   int        `json:"per_user_limit"` // 0 = без ограничения
	IsActive       bool       `json:"is_active"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Usable — код в принципе действует (без учёта персональных лимитов клиента
// и суммы конкретного заказа).
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

// MeetsMinimum — сумма заказа дотягивает до порога промокода.
func (p *PromoCode) MeetsMinimum(subtotal int) bool {
	return p.MinOrderAmount <= 0 || subtotal >= p.MinOrderAmount
}

// Apply считает скидку и итог по правилу промокода. Единственное место, где
// скидка превращается в рубли: и заказ, и предварительная проверка на витрине
// зовут его, поэтому разойтись они не могут.
func (p *PromoCode) Apply(subtotal int) (discount, total int) {
	if subtotal <= 0 {
		return 0, max(subtotal, 0)
	}
	if p.DiscountType == DiscountTypeFixed {
		// Скидка не может увести заказ в минус: за доставку букета
		// магазин не доплачивает.
		discount = min(max(p.DiscountValue, 0), subtotal)
		return discount, subtotal - discount
	}
	return ApplyDiscount(subtotal, p.DiscountValue)
}

// Percent — процент скидки для витрины; у фиксированных кодов 0.
func (p *PromoCode) Percent() int {
	if p.DiscountType == DiscountTypeFixed {
		return 0
	}
	return p.DiscountValue
}

// Describe — правило скидки одной строкой, для чата админа и уведомлений.
func (p *PromoCode) Describe() string {
	if p.DiscountType == DiscountTypeFixed {
		return strconv.Itoa(p.DiscountValue) + " ₽"
	}
	return strconv.Itoa(p.DiscountValue) + "%"
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
