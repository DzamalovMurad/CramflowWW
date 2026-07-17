package repository

import (
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

type Repository struct {
	DB *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

// --- Products ---

// Значения быстрых фильтров каталога.
const (
	FilterPopular  = "popular"  // 🔥 Популярное — по числу проданных единиц
	FilterNew      = "new"      // 🆕 Новинки — добавлены за последние 14 дней
	FilterPreorder = "preorder" // 📅 Предзаказ — любой букет к выбранной дате и времени
	FilterBudget   = "budget"   // 💰 До 3000 ₽ — минимальный вариант не дороже 3000
)

func (r *Repository) ListProducts(category, filter, search string) ([]model.Product, error) {
	q := r.DB.Preload("Variants", func(db *gorm.DB) *gorm.DB {
		return db.Order("price ASC")
	}).Preload("Images").Where("is_hidden = ?", false)

	if category != "" {
		q = q.Where("category = ?", category)
	}

	if search = strings.TrimSpace(search); search != "" {
		q = q.Where("name ILIKE ?", "%"+search+"%")
	}

	switch filter {
	case FilterNew:
		q = q.Where("products.created_at > ?", time.Now().AddDate(0, 0, -14))
	case FilterBudget:
		q = q.Where("(SELECT MIN(price) FROM product_variants v WHERE v.product_id = products.id) <= ?", 3000)
	case FilterPopular:
		q = q.Order(`(SELECT COALESCE(SUM(oi.quantity), 0)
			FROM order_items oi
			JOIN product_variants v ON v.id = oi.variant_id
			WHERE v.product_id = products.id) DESC`)
	case FilterPreorder:
		// Предзаказ доступен для всего каталога: дату и время клиент выбирает в checkout.
	}

	q = q.Order("products.created_at DESC")

	var products []model.Product
	err := q.Find(&products).Error
	return products, err
}

// ListAllProducts — для админ-бота, включая скрытые.
func (r *Repository) ListAllProducts() ([]model.Product, error) {
	var products []model.Product
	err := r.DB.Preload("Variants").Preload("Images").Order("id DESC").Find(&products).Error
	return products, err
}

func (r *Repository) GetProduct(id uint) (*model.Product, error) {
	var p model.Product
	err := r.DB.Preload("Variants", func(db *gorm.DB) *gorm.DB {
		return db.Order("price ASC")
	}).Preload("Images").First(&p, id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) CreateProduct(p *model.Product) error {
	return r.DB.Create(p).Error
}

func (r *Repository) SaveProduct(p *model.Product) error {
	return r.DB.Save(p).Error
}

// ReplaceVariants заменяет все варианты товара.
func (r *Repository) ReplaceVariants(productID uint, variants []model.ProductVariant) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("product_id = ?", productID).Delete(&model.ProductVariant{}).Error; err != nil {
			return err
		}
		for i := range variants {
			variants[i].ProductID = productID
		}
		return tx.Create(&variants).Error
	})
}

// ReplaceImages заменяет все фото товара.
func (r *Repository) ReplaceImages(productID uint, urls []string) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("product_id = ?", productID).Delete(&model.ProductImage{}).Error; err != nil {
			return err
		}
		images := make([]model.ProductImage, 0, len(urls))
		for _, u := range urls {
			images = append(images, model.ProductImage{ProductID: productID, URL: u})
		}
		if len(images) == 0 {
			return nil
		}
		return tx.Create(&images).Error
	})
}

func (r *Repository) SetProductHidden(id uint, hidden bool) error {
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("is_hidden", hidden).Error
}

func (r *Repository) SetProductHit(id uint, hit bool) error {
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("is_hit", hit).Error
}

func (r *Repository) SetProductStock(id uint, stock int) error {
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("stock", stock).Error
}

// SetProductDiscount проставляет старую цену вариантов из текущей и процента скидки
// (percent 0 — убрать скидку). Бейдж −N% на витрине считается по old_price/price.
func (r *Repository) SetProductDiscount(id uint, percent int) error {
	var variants []model.ProductVariant
	if err := r.DB.Where("product_id = ?", id).Find(&variants).Error; err != nil {
		return err
	}
	return r.DB.Transaction(func(tx *gorm.DB) error {
		for _, v := range variants {
			old := 0
			if percent > 0 && percent < 100 {
				old = v.Price * 100 / (100 - percent) // цена «до скидки»
			}
			if err := tx.Model(&model.ProductVariant{}).Where("id = ?", v.ID).
				Update("old_price", old).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) DeleteProduct(id uint) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("product_id = ?", id).Delete(&model.ProductVariant{}).Error; err != nil {
			return err
		}
		if err := tx.Where("product_id = ?", id).Delete(&model.ProductImage{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Product{}, id).Error
	})
}

func (r *Repository) GetVariant(id uint) (*model.ProductVariant, error) {
	var v model.ProductVariant
	if err := r.DB.First(&v, id).Error; err != nil {
		return nil, err
	}
	return &v, nil
}

// --- Users ---

// UpsertUser находит или создаёт пользователя по telegram_id и обновляет имя/телефон.
func (r *Repository) UpsertUser(telegramID int64, name, phone string) (*model.User, error) {
	var u model.User
	err := r.DB.Where(model.User{TelegramID: telegramID}).FirstOrCreate(&u).Error
	if err != nil {
		return nil, err
	}
	changed := false
	if name != "" && u.Name != name {
		u.Name = name
		changed = true
	}
	if phone != "" && u.Phone != phone {
		u.Phone = phone
		changed = true
	}
	if changed {
		if err := r.DB.Save(&u).Error; err != nil {
			return nil, err
		}
	}
	return &u, nil
}

func (r *Repository) GetUserByTelegramID(telegramID int64) (*model.User, error) {
	var u model.User
	err := r.DB.Preload("PromoCode").Where("telegram_id = ?", telegramID).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *Repository) SetUserPromo(userID uint, promoID uint) error {
	return r.DB.Model(&model.User{}).Where("id = ?", userID).Update("promo_code_id", promoID).Error
}

// --- Promo codes ---

func (r *Repository) GetPromoByCode(code string) (*model.PromoCode, error) {
	var p model.PromoCode
	err := r.DB.Where("UPPER(code) = UPPER(?)", code).First(&p).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) GetPromoByID(id uint) (*model.PromoCode, error) {
	var p model.PromoCode
	if err := r.DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) IncrementPromoUses(id uint) error {
	return r.DB.Model(&model.PromoCode{}).Where("id = ?", id).
		Update("uses", gorm.Expr("uses + 1")).Error
}

// --- Orders ---

func (r *Repository) CreateOrder(o *model.Order) error {
	return r.DB.Create(o).Error
}

func (r *Repository) GetOrder(id uint) (*model.Order, error) {
	var o model.Order
	err := r.DB.Preload("User").Preload("PromoCode").
		Preload("Items").Preload("Items.Variant").
		First(&o, id).Error
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *Repository) ListRecentOrders(limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.DB.Preload("User").Order("id DESC").Limit(limit).Find(&orders).Error
	return orders, err
}

func (r *Repository) UpdateOrderStatus(id uint, status string) error {
	return r.DB.Model(&model.Order{}).Where("id = ?", id).Update("status", status).Error
}

// --- «Сегодня на базе» ---

// UpsertFreshToday сохраняет список свежих цветов за дату (перезаписывает существующий).
func (r *Repository) UpsertFreshToday(date, items string) error {
	var f model.FreshToday
	err := r.DB.Where("date = ?", date).First(&f).Error
	if err != nil {
		return r.DB.Create(&model.FreshToday{Date: date, Items: items}).Error
	}
	f.Items = items
	return r.DB.Save(&f).Error
}

func (r *Repository) GetFreshToday(date string) (*model.FreshToday, error) {
	var f model.FreshToday
	if err := r.DB.Where("date = ?", date).First(&f).Error; err != nil {
		return nil, err
	}
	return &f, nil
}
