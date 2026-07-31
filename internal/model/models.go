package model

import (
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

type Product struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"not null" json:"name"`
	Description string `json:"description"`
	Category    string `gorm:"not null;index" json:"category"`
	IsHidden    bool   `gorm:"not null;default:false;index" json:"is_hidden"`
	// IsAvailable — оперативное «есть/нет в наличии» (/stock в боте).
	// Отдельный от IsHidden флаг: IsHidden — сезонное решение «убрать с витрины»,
	// IsAvailable — сегодняшняя закупка. Витрина требует обоих: показан и в наличии.
	IsAvailable bool `gorm:"not null;default:true;index" json:"is_available"`
	IsHit       bool `gorm:"not null;default:false" json:"is_hit"` // бейдж «ХИТ»
	Stock       int  `gorm:"not null;default:0" json:"stock"`      // остаток (0 = не показывать «осталось N»)
	// LowStock — ручной бейдж «мало осталось»: флорист видит остаток на базе,
	// но пересчитывать штуки в Stock ему лень. Ставится тумблером в /edit.
	LowStock bool `gorm:"not null;default:false" json:"low_stock"`
	// SortOrder — порядок в подборках (сейчас — «хиты» для пустой корзины).
	// Меньше = выше; при равенстве побеждает более новый товар.
	SortOrder int `gorm:"not null;default:0" json:"sort_order"`
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
	// AcquisitionSource — first-touch источник (startapp-параметр первого запуска
	// Mini App: product_<id>, метка src_<tag> и т.п.). Записывается один раз.
	AcquisitionSource string `json:"acquisition_source,omitempty"`
	// Промокод, полученный по deep-link t.me/bot?start=CODE.
	PromoCodeID *uint      `json:"promo_code_id,omitempty"`
	PromoCode   *PromoCode `json:"promo_code,omitempty"`
	// BotBlocked — клиент заблокировал бота (Telegram вернул 403 при рассылке).
	// Такие адресаты исключаются из следующих рассылок, чтобы не жечь лимиты.
	BotBlocked bool `gorm:"not null;default:false;index" json:"bot_blocked"`
	// LastCartAt — последняя активность в корзине Mini App (POST /api/cart/touch).
	// По ней собирается сегмент «корзина без заказа».
	LastCartAt *time.Time `gorm:"index" json:"last_cart_at,omitempty"`
	CreatedAt  time.Time  `gorm:"index" json:"created_at"`
}

type Order struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	UserID          uint   `gorm:"not null;index;index:idx_orders_user_status" json:"user_id"`
	TotalPrice      int    `gorm:"not null" json:"total_price"`
	DeliveryAddress string `gorm:"not null" json:"delivery_address"`
	DeliveryDate    string `gorm:"not null;index:idx_orders_status_ddate" json:"delivery_date"`
	DeliveryTime    string `gorm:"not null" json:"delivery_time"`
	PromoCodeID     *uint  `json:"promo_code_id,omitempty"`
	Comment         string `json:"comment"`
	CardText        string `json:"card_text"`     // текст открытки (до 300 символов)
	IsAnonymous     bool   `json:"is_anonymous"`  // анонимная доставка
	CancelReason    string `json:"cancel_reason"` // причина отмены (обязательна при отмене админом)
	// Получатель, если букет везут не заказчику (в CSV-выгрузке — отдельные колонки).
	RecipientName  string `json:"recipient_name"`
	RecipientPhone string `json:"recipient_phone"`
	// Source — канал привлечения заказа. Обе ветки писали сюда одно и то же:
	// метку из start_param Mini App (ветка фото/уведомлений) либо из ?src=
	// в ссылке (ветка админ-бота). Колонка одна и NOT NULL: /stats группирует
	// по ней, и «пустой» источник в отчёте — это всегда SourceDirect, а не
	// отдельная безымянная строка. Значение клиентское → NormalizeSource.
	Source    string    `gorm:"not null;default:direct;index:idx_orders_created_source,priority:2" json:"source"`
	Status    string    `gorm:"not null;default:new;index;index:idx_orders_status_ddate,priority:1;index:idx_orders_user_status,priority:2;index:idx_orders_created_status,priority:2" json:"status"`
	CreatedAt time.Time `gorm:"index:idx_orders_created_status,priority:1;index:idx_orders_created_source,priority:1" json:"created_at"`

	User      User         `json:"user"`
	PromoCode *PromoCode   `json:"promo_code,omitempty"`
	Items     []OrderItem  `gorm:"constraint:OnDelete:CASCADE" json:"items"`
	Photos    []OrderPhoto `gorm:"constraint:OnDelete:CASCADE" json:"photos,omitempty"`
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

// Setting — key/value настроек рантайма, которые админ переключает из бота
// и которые обязаны пережить рестарт контейнера (fallback-режим заказов).
type Setting struct {
	Key       string    `gorm:"primaryKey" json:"key"`
	Value     string    `gorm:"not null" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Ключи настроек.
const (
	// SettingFallbackOrders — «on», если заказ можно оформить диалогом в боте
	// (Mini App недоступен). Переключается командой /fallback on|off.
	SettingFallbackOrders = "fallback_orders"
)

type PromoCode struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	Code            string `gorm:"uniqueIndex;not null" json:"code"`
	DiscountPercent int    `gorm:"not null" json:"discount_percent"`
	Uses            int    `gorm:"not null;default:0" json:"uses"`
	// MaxUses — лимит применений (0 = без лимита). Одноразовые коды за отзыв
	// и подписку создаются с MaxUses=1; исчерпанный код не находится по /api/promo.
	MaxUses int `gorm:"not null;default:0" json:"max_uses"`
}

// Типы фото букета по заказу.
const (
	PhotoAssembled = "assembled" // букет собран
	PhotoDelivered = "delivered" // букет вручён
)

// PhotoTypeForStatus — какой тип фото снимает админ на текущем шаге конвейера:
// до передачи курьеру — «собран», начиная с доставки — «вручён».
func PhotoTypeForStatus(status string) string {
	if status == StatusDelivering || status == StatusDelivered {
		return PhotoDelivered
	}
	return PhotoAssembled
}

// OrderPhoto — фото букета по заказу. Храним только telegram file_id:
// файл живёт на серверах Telegram, пересылается клиенту без скачивания.
type OrderPhoto struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	OrderID uint   `gorm:"not null;index" json:"order_id"`
	Type    string `gorm:"not null" json:"type"` // assembled | delivered
	FileID  string `gorm:"not null" json:"file_id"`
	// IsDocument — фото прислано файлом (file_id документа нельзя отправить
	// через sendPhoto — только через sendDocument, и наоборот).
	IsDocument bool      `gorm:"not null;default:false" json:"is_document"`
	CreatedAt  time.Time `json:"created_at"`
}

// Типы отложенных уведомлений.
const (
	NotificationFeedback = "feedback" // просьба об отзыве через 2ч после доставки
)

// Notification — отложенное уведомление. Планируется в БД (due_at), поэтому
// перезапуск сервиса ничего не теряет; sent_at — защита от повторной отправки.
type Notification struct {
	ID      uint       `gorm:"primaryKey" json:"id"`
	OrderID uint       `gorm:"not null;uniqueIndex:idx_notifications_order_type" json:"order_id"`
	Type    string     `gorm:"not null;uniqueIndex:idx_notifications_order_type" json:"type"`
	DueAt   time.Time  `gorm:"not null;index" json:"due_at"`
	SentAt  *time.Time `gorm:"index" json:"sent_at,omitempty"`
	// RespondedAt — клиент ответил на просьбу об отзыве (промокод уже выдан).
	RespondedAt *time.Time `json:"responded_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Типы разовых бонусов.
const (
	BonusSubscription = "subscription" // промокод за подписку на канал
)

// ClaimedBonus — выданные разовые бонусы: уникальный индекс (user_id, type)
// не даёт получить один бонус дважды (защита от фарминга).
type ClaimedBonus struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"not null;uniqueIndex:idx_claimed_bonuses_user_type" json:"user_id"`
	Type        string    `gorm:"not null;uniqueIndex:idx_claimed_bonuses_user_type" json:"type"`
	PromoCodeID uint      `gorm:"not null" json:"promo_code_id"`
	CreatedAt   time.Time `json:"created_at"`

	PromoCode PromoCode `json:"promo_code"`
}
