package model

import "time"

// Категории товаров.
const (
	CategoryStandard = "Стандарт"
	CategoryPremium  = "Премиум"
	CategoryLux      = "Люкс"
	CategoryWow      = "WOW"
)

var Categories = []string{CategoryStandard, CategoryPremium, CategoryLux, CategoryWow}

// Статусы заказов.
const (
	StatusNew        = "new"
	StatusConfirmed  = "confirmed"
	StatusDelivering = "delivering"
	StatusDone       = "done"
	StatusCancelled  = "cancelled"
)

var StatusLabels = map[string]string{
	StatusNew:        "🆕 Новый",
	StatusConfirmed:  "✅ Подтверждён",
	StatusDelivering: "🚚 Доставляется",
	StatusDone:       "🏁 Выполнен",
	StatusCancelled:  "❌ Отменён",
}

var StatusOrder = []string{StatusNew, StatusConfirmed, StatusDelivering, StatusDone, StatusCancelled}

type Product struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Name        string `gorm:"not null" json:"name"`
	Description string `json:"description"`
	Category    string `gorm:"not null;index" json:"category"`
	IsHidden    bool   `gorm:"not null;default:false;index" json:"is_hidden"`
	CreatedAt   time.Time `json:"created_at"`

	Variants []ProductVariant `gorm:"constraint:OnDelete:CASCADE" json:"variants,omitempty"`
	Images   []ProductImage   `gorm:"constraint:OnDelete:CASCADE" json:"images,omitempty"`
}

type ProductVariant struct {
	ID        uint `gorm:"primaryKey" json:"id"`
	ProductID uint `gorm:"not null;index" json:"product_id"`
	Quantity  int  `gorm:"not null" json:"quantity"` // кол-во цветов в букете
	Price     int  `gorm:"not null" json:"price"`    // цена в рублях
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
	ID              uint   `gorm:"primaryKey" json:"id"`
	UserID          uint   `gorm:"not null;index" json:"user_id"`
	TotalPrice      int    `gorm:"not null" json:"total_price"`
	DeliveryAddress string `gorm:"not null" json:"delivery_address"`
	DeliveryDate    string `gorm:"not null" json:"delivery_date"`
	DeliveryTime    string `gorm:"not null" json:"delivery_time"`
	PromoCodeID     *uint  `json:"promo_code_id,omitempty"`
	Comment         string `json:"comment"`
	Status          string `gorm:"not null;default:new;index" json:"status"`
	CreatedAt       time.Time `json:"created_at"`

	User      User        `json:"user"`
	PromoCode *PromoCode  `json:"promo_code,omitempty"`
	Items     []OrderItem `gorm:"constraint:OnDelete:CASCADE" json:"items"`
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

type PromoCode struct {
	ID              uint   `gorm:"primaryKey" json:"id"`
	Code            string `gorm:"uniqueIndex;not null" json:"code"`
	DiscountPercent int    `gorm:"not null" json:"discount_percent"`
	Uses            int    `gorm:"not null;default:0" json:"uses"`
}
