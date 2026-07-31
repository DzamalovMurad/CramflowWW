package repository

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

type Repository struct {
	DB *gorm.DB

	// In-memory кэш витрины: полный список видимых товаров.
	// TTL + инвалидация при любом изменении ассортимента ботом.
	catMu    sync.RWMutex
	catItems []model.Product
	catAt    time.Time
}

const catalogTTL = 5 * time.Minute

// catalogVisible — единственное определение «товар виден на витрине»:
// не скрыт сезонно, есть в наличии, не удалён. Используется всеми выборками
// каталога, чтобы /stock мгновенно убирал товар отовсюду, а не из одного места.
// Под это условие заведён частичный индекс idx_products_catalog (миграция 003).
const catalogVisible = "is_hidden = FALSE AND is_available = TRUE AND archived_at IS NULL"

// catalogVisibleOn — то же условие с явным алиасом таблицы. Нужен в запросах
// с JOIN: archived_at есть и у products, и у product_variants, без префикса
// Postgres справедливо ругается на неоднозначную колонку.
func catalogVisibleOn(alias string) string {
	return fmt.Sprintf("%[1]s.is_hidden = FALSE AND %[1]s.is_available = TRUE AND %[1]s.archived_at IS NULL", alias)
}

func New(db *gorm.DB) *Repository {
	return &Repository{DB: db}
}

// InvalidateCatalog сбрасывает кэш витрины (вызывается после правок товаров).
func (r *Repository) InvalidateCatalog() {
	r.catMu.Lock()
	r.catItems = nil
	r.catMu.Unlock()
}

// visibleProducts возвращает все видимые товары из кэша (или из БД с прогревом).
func (r *Repository) visibleProducts() ([]model.Product, error) {
	r.catMu.RLock()
	if r.catItems != nil && time.Since(r.catAt) < catalogTTL {
		items := r.catItems
		r.catMu.RUnlock()
		return items, nil
	}
	r.catMu.RUnlock()

	var products []model.Product
	err := r.DB.Preload("Variants", func(db *gorm.DB) *gorm.DB {
		return db.Where("archived_at IS NULL").Order("price ASC")
	}).Preload("Images").
		Where(catalogVisible).
		Order("created_at DESC").
		Find(&products).Error
	if err != nil {
		return nil, err
	}

	r.catMu.Lock()
	r.catItems = products
	r.catAt = time.Now()
	r.catMu.Unlock()
	return products, nil
}

// --- Products ---

// Значения быстрых фильтров каталога.
const (
	FilterPopular  = "popular"  // 🔥 Популярное — по числу проданных единиц
	FilterNew      = "new"      // 🆕 Новинки — добавлены за последние 14 дней
	FilterPreorder = "preorder" // 📅 Предзаказ — любой букет к выбранной дате и времени
	FilterBudget   = "budget"   // 💰 До 3000 ₽ — минимальный вариант не дороже 3000
)

// ListProducts отдаёт витрину из in-memory кэша: категория, быстрые фильтры
// и нечёткий поиск применяются в памяти. Исключение — «популярное»:
// сортировка по продажам требует SQL-агрегации и идёт мимо кэша.
func (r *Repository) ListProducts(category, filter, search string) ([]model.Product, error) {
	search = strings.TrimSpace(search)

	if filter == FilterPopular {
		return r.listPopular(category, search)
	}

	all, err := r.visibleProducts()
	if err != nil {
		return nil, err
	}

	cutoff := time.Now().AddDate(0, 0, -14)
	out := make([]model.Product, 0, len(all))
	for _, p := range all {
		if category != "" && p.Category != category {
			continue
		}
		switch filter {
		case FilterNew:
			if !p.CreatedAt.After(cutoff) {
				continue
			}
		case FilterBudget:
			if len(p.Variants) == 0 || p.Variants[0].Price > 3000 {
				continue
			}
		case FilterPreorder:
			// Предзаказ доступен для всего каталога: дату и время клиент выбирает в checkout.
		}
		if !MatchesSearch(p.Name, search) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func (r *Repository) listPopular(category, search string) ([]model.Product, error) {
	q := r.DB.Preload("Variants", func(db *gorm.DB) *gorm.DB {
		return db.Where("archived_at IS NULL").Order("price ASC")
	}).Preload("Images").Where(catalogVisible)
	if category != "" {
		q = q.Where("category = ?", category)
	}
	q = q.Order(`(SELECT COALESCE(SUM(oi.quantity), 0)
		FROM order_items oi
		JOIN product_variants v ON v.id = oi.variant_id
		WHERE v.product_id = products.id) DESC`).
		Order("products.created_at DESC")

	var products []model.Product
	if err := q.Find(&products).Error; err != nil {
		return nil, err
	}
	if search == "" {
		return products, nil
	}
	out := make([]model.Product, 0, len(products))
	for _, p := range products {
		if MatchesSearch(p.Name, search) {
			out = append(out, p)
		}
	}
	return out, nil
}

// ListAllProducts — для админ-бота: включая скрытые, но без архивных
// (архивные = «удалённые», их незачем показывать в /edit, /hide, /delete).
func (r *Repository) ListAllProducts() ([]model.Product, error) {
	var products []model.Product
	err := r.DB.Preload("Variants", func(db *gorm.DB) *gorm.DB {
		return db.Where("archived_at IS NULL").Order("price ASC")
	}).Preload("Images").
		Where("archived_at IS NULL").
		Order("id DESC").Find(&products).Error
	return products, err
}

func (r *Repository) GetProduct(id uint) (*model.Product, error) {
	var p model.Product
	err := r.DB.Preload("Variants", func(db *gorm.DB) *gorm.DB {
		return db.Where("archived_at IS NULL").Order("price ASC")
	}).Preload("Images").First(&p, id).Error
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) CreateProduct(p *model.Product) error {
	defer r.InvalidateCatalog()
	return r.DB.Create(p).Error
}

func (r *Repository) SaveProduct(p *model.Product) error {
	defer r.InvalidateCatalog()
	return r.DB.Save(p).Error
}

// ReplaceVariants заменяет варианты товара новым набором.
// Старые варианты не удаляются жёстко, а архивируются: на них могут ссылаться
// order_items прошлых заказов (FK fk_order_items_variant). Не использованные
// нигде варианты подчищаем физически, чтобы таблица не пухла.
func (r *Repository) ReplaceVariants(productID uint, variants []model.ProductVariant) error {
	defer r.InvalidateCatalog()
	now := time.Now()
	return r.DB.Transaction(func(tx *gorm.DB) error {
		// 1. Архивируем текущие варианты — с витрины исчезают сразу.
		if err := tx.Model(&model.ProductVariant{}).
			Where("product_id = ? AND archived_at IS NULL", productID).
			Update("archived_at", &now).Error; err != nil {
			return err
		}
		// 2. Физически удаляем те архивные, что не встречаются ни в одном заказе.
		if err := tx.Where(`product_id = ? AND archived_at IS NOT NULL
			AND id NOT IN (SELECT variant_id FROM order_items)`, productID).
			Delete(&model.ProductVariant{}).Error; err != nil {
			return err
		}
		// 3. Пишем новый набор.
		for i := range variants {
			variants[i].ProductID = productID
			variants[i].ArchivedAt = nil
		}
		if len(variants) == 0 {
			return nil
		}
		return tx.Create(&variants).Error
	})
}

// ReplaceImages заменяет все фото товара.
func (r *Repository) ReplaceImages(productID uint, urls []string) error {
	defer r.InvalidateCatalog()
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
	defer r.InvalidateCatalog()
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("is_hidden", hidden).Error
}

// SetProductAvailable — «есть/нет в наличии» из /stock. Сбрасывает кэш витрины,
// поэтому товар пропадает из Mini App сразу, а не через TTL.
// Прошлые заказы не трогает: они читаются через order_items, а не через каталог.
func (r *Repository) SetProductAvailable(id uint, available bool) error {
	defer r.InvalidateCatalog()
	res := r.DB.Model(&model.Product{}).Where("id = ?", id).
		Update("is_available", available)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// CountLiveProducts — сколько товаров показывает /stock (для пагинации).
func (r *Repository) CountLiveProducts() (int64, error) {
	var n int64
	err := r.DB.Model(&model.Product{}).Where("archived_at IS NULL").Count(&n).Error
	return n, err
}

// ListProductsPage — страница товаров для /stock: без вариантов и фото,
// боту нужны только имя и флаги. Порядок стабильный, иначе товар прыгает
// между страницами после переключения наличия.
func (r *Repository) ListProductsPage(page, per int) ([]model.Product, error) {
	if page < 1 {
		page = 1
	}
	var products []model.Product
	err := r.DB.Where("archived_at IS NULL").
		Order("id DESC").
		Limit(per).Offset((page - 1) * per).
		Find(&products).Error
	return products, err
}

// UnavailableVariants — какие из переданных вариантов больше нельзя заказать
// (товар скрыт, снят с наличия или удалён, либо сам вариант архивирован).
// Один запрос на всю корзину — Mini App зовёт его перед оформлением.
func (r *Repository) UnavailableVariants(variantIDs []uint) ([]uint, error) {
	if len(variantIDs) == 0 {
		return nil, nil
	}
	var rows []struct {
		ID uint
		OK bool
	}
	err := r.DB.Raw(`
		SELECT v.id,
		       (v.archived_at IS NULL AND `+catalogVisibleOn("p")+`) AS ok
		FROM product_variants v
		JOIN products p ON p.id = v.product_id
		WHERE v.id IN ?`, variantIDs).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	live := make(map[uint]bool, len(rows))
	for _, row := range rows {
		live[row.ID] = row.OK
	}
	// Вариант, которого в БД нет вовсе (подделан клиентом или вычищен),
	// тоже недоступен — иначе он молча уедет в заказ.
	var out []uint
	for _, id := range variantIDs {
		if !live[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

func (r *Repository) SetProductHit(id uint, hit bool) error {
	defer r.InvalidateCatalog()
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("is_hit", hit).Error
}

func (r *Repository) SetProductStock(id uint, stock int) error {
	defer r.InvalidateCatalog()
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("stock", stock).Error
}

// SetProductDiscount проставляет старую цену вариантов из текущей и процента скидки
// (percent 0 — убрать скидку). Бейдж −N% на витрине считается по old_price/price.
func (r *Repository) SetProductDiscount(id uint, percent int) error {
	defer r.InvalidateCatalog()
	var variants []model.ProductVariant
	if err := r.DB.Where("product_id = ? AND archived_at IS NULL", id).Find(&variants).Error; err != nil {
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

// DeleteProduct — «удаление» товара из каталога через soft delete.
// Жёсткий DELETE невозможен: order_items ссылается на product_variants
// (FK fk_order_items_variant), а история заказов должна оставаться читаемой.
// Товар помечается скрытым и archived_at — витрина его не отдаёт,
// прошлые заказы продолжают корректно отображаться.
func (r *Repository) DeleteProduct(id uint) error {
	defer r.InvalidateCatalog()
	now := time.Now()
	res := r.DB.Model(&model.Product{}).Where("id = ?", id).
		Updates(map[string]any{"is_hidden": true, "archived_at": &now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// GetVariant — только живой вариант: архивный заказать нельзя
// (корзина клиента могла устареть после правки цен).
func (r *Repository) GetVariant(id uint) (*model.ProductVariant, error) {
	var v model.ProductVariant
	if err := r.DB.Where("archived_at IS NULL").First(&v, id).Error; err != nil {
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

// GetPromoByCode — живой промокод: исчерпанные одноразовые (uses >= max_uses)
// ведут себя как несуществующие.
func (r *Repository) GetPromoByCode(code string) (*model.PromoCode, error) {
	var p model.PromoCode
	err := r.DB.Where("UPPER(code) = UPPER(?)", code).
		Where("max_uses = 0 OR uses < max_uses").
		First(&p).Error
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
