// Package repository — доступ к БД. Все методы принимают context:
// он несёт таймаут запроса и отменяет зависший SQL вместе с HTTP-запросом.
package repository

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// ErrNotFound — запись не найдена. Обёртка над gorm.ErrRecordNotFound,
// чтобы вызывающий код не знал про GORM.
var ErrNotFound = errors.New("запись не найдена")

// catalogCache — общий на все копии Repository (в т.ч. транзакционные) кэш
// витрины и рейтинга продаж.
type catalogCache struct {
	mu    sync.RWMutex
	items []model.Product
	at    time.Time

	// sales: product_id → сколько штук продано. Считать это подзапросом на
	// каждый запрос было дорого (десятки миллисекунд на годовом объёме
	// заказов), а меняется рейтинг медленно — держим в кэше.
	sales   map[uint]int64
	salesAt time.Time
}

const (
	catalogTTL = 60 * time.Second
	salesTTL   = 5 * time.Minute
)

type Repository struct {
	DB    *gorm.DB
	cache *catalogCache
	now   func() time.Time
}

// New создаёт репозиторий. now — часы магазина (см. config.Config.Now).
func New(db *gorm.DB, now func() time.Time) *Repository {
	if now == nil {
		now = time.Now
	}
	return &Repository{DB: db, cache: &catalogCache{}, now: now}
}

// Tx выполняет fn в одной транзакции. Внутри fn нужно пользоваться
// переданным репозиторием — он привязан к транзакции.
func (r *Repository) Tx(ctx context.Context, fn func(*Repository) error) error {
	return r.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&Repository{DB: tx, cache: r.cache, now: r.now})
	})
}

func (r *Repository) db(ctx context.Context) *gorm.DB { return r.DB.WithContext(ctx) }

// wrap переводит ошибки GORM в доменные.
func wrap(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}

// InvalidateCatalog сбрасывает кэш витрины (после любой правки ассортимента).
func (r *Repository) InvalidateCatalog() {
	r.cache.mu.Lock()
	r.cache.items = nil
	r.cache.mu.Unlock()
}

// salesRank — сколько штук продано по каждому товару. Обновляется раз в
// salesTTL: порядок «популярного» не обязан быть секунда-в-секунду точным.
func (r *Repository) salesRank(ctx context.Context) (map[uint]int64, error) {
	r.cache.mu.RLock()
	if r.cache.sales != nil && time.Since(r.cache.salesAt) < salesTTL {
		sales := r.cache.sales
		r.cache.mu.RUnlock()
		return sales, nil
	}
	r.cache.mu.RUnlock()

	var rows []struct {
		ProductID uint
		Sold      int64
	}
	err := r.db(ctx).Raw(`
		SELECT v.product_id, SUM(oi.quantity) AS sold
		FROM order_items oi
		JOIN product_variants v ON v.id = oi.variant_id
		GROUP BY v.product_id`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("рейтинг продаж: %w", err)
	}
	sales := make(map[uint]int64, len(rows))
	for _, row := range rows {
		sales[row.ProductID] = row.Sold
	}

	r.cache.mu.Lock()
	r.cache.sales = sales
	r.cache.salesAt = time.Now()
	r.cache.mu.Unlock()
	return sales, nil
}

// liveVariants / images — общие для всех выборок правила подгрузки.
func liveVariants(db *gorm.DB) *gorm.DB {
	return db.Where("archived_at IS NULL").Order("price ASC")
}

func orderedImages(db *gorm.DB) *gorm.DB {
	// Порядок фото — порядок загрузки; без ORDER BY Postgres его не гарантирует.
	return db.Order("id ASC")
}

// visibleProducts — все живые товары витрины (из кэша или из БД).
func (r *Repository) visibleProducts(ctx context.Context) ([]model.Product, error) {
	r.cache.mu.RLock()
	if r.cache.items != nil && time.Since(r.cache.at) < catalogTTL {
		items := r.cache.items
		r.cache.mu.RUnlock()
		return items, nil
	}
	r.cache.mu.RUnlock()

	var products []model.Product
	err := r.db(ctx).
		Preload("Variants", liveVariants).
		Preload("Images", orderedImages).
		Where("is_hidden = FALSE AND archived_at IS NULL").
		Order("created_at DESC, id DESC").
		Find(&products).Error
	if err != nil {
		return nil, fmt.Errorf("витрина: %w", err)
	}

	r.cache.mu.Lock()
	r.cache.items = products
	r.cache.at = time.Now()
	r.cache.mu.Unlock()
	return products, nil
}

// ─── Товары ────────────────────────────────────────────────────────────────

// Значения быстрых фильтров каталога.
const (
	FilterPopular  = "popular"  // по числу проданных единиц
	FilterNew      = "new"      // добавлены за последние 14 дней
	FilterPreorder = "preorder" // любой букет к выбранной дате
	FilterBudget   = "budget"   // минимальный вариант не дороже 3000 ₽
)

const budgetMaxPrice = 3000

// ListProducts отдаёт витрину: категория, быстрый фильтр и нечёткий поиск
// применяются в памяти поверх кэша. Исключение — «популярное»: сортировка
// по продажам требует агрегации в SQL.
func (r *Repository) ListProducts(ctx context.Context, category, filter, search string) ([]model.Product, error) {
	search = strings.TrimSpace(search)

	if filter == FilterPopular {
		return r.listPopular(ctx, category, search)
	}

	cutoff := r.now().AddDate(0, 0, -14)
	keep := func(p model.Product) bool {
		switch filter {
		case FilterNew:
			return p.CreatedAt.After(cutoff)
		case FilterBudget:
			return len(p.Variants) > 0 && p.Variants[0].Price <= budgetMaxPrice
		default:
			// Предзаказ доступен для всего каталога: дату клиент выбирает в checkout.
			return true
		}
	}
	return r.filterVisible(ctx, category, search, keep)
}

// listPopular сортирует ту же закэшированную витрину по числу продаж.
// Раньше здесь был коррелированный подзапрос в ORDER BY: он выполнялся для
// каждого товара при каждом запросе и на годовом объёме заказов стоил
// десятки миллисекунд. Теперь — один агрегат раз в пять минут.
func (r *Repository) listPopular(ctx context.Context, category, search string) ([]model.Product, error) {
	products, err := r.filterVisible(ctx, category, search, func(model.Product) bool { return true })
	if err != nil {
		return nil, err
	}
	sales, err := r.salesRank(ctx)
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(products, func(a, b model.Product) int {
		if d := cmp.Compare(sales[b.ID], sales[a.ID]); d != 0 {
			return d
		}
		return b.CreatedAt.Compare(a.CreatedAt) // при равных продажах — новее выше
	})
	return products, nil
}

// filterVisible — общая часть всех выборок витрины: категория, поиск и
// произвольное дополнительное условие поверх закэшированного списка.
func (r *Repository) filterVisible(ctx context.Context, category, search string, keep func(model.Product) bool) ([]model.Product, error) {
	all, err := r.visibleProducts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]model.Product, 0, len(all))
	for _, p := range all {
		if category != "" && p.Category != category {
			continue
		}
		if !keep(p) {
			continue
		}
		if !MatchesSearch(p.Name, search) {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// ListAllProducts — для админ-бота: включая скрытые, но без архивных
// («удалённые» незачем показывать в /edit, /hide, /delete).
func (r *Repository) ListAllProducts(ctx context.Context) ([]model.Product, error) {
	var products []model.Product
	err := r.db(ctx).
		Preload("Variants", liveVariants).
		Preload("Images", orderedImages).
		Where("archived_at IS NULL").
		Order("id DESC").Find(&products).Error
	return products, wrap(err)
}

// GetProduct возвращает товар вместе с живыми вариантами и фото.
// Архивные (удалённые) товары не отдаются вовсе.
func (r *Repository) GetProduct(ctx context.Context, id uint) (*model.Product, error) {
	var p model.Product
	err := r.db(ctx).
		Preload("Variants", liveVariants).
		Preload("Images", orderedImages).
		Where("archived_at IS NULL").
		First(&p, id).Error
	if err != nil {
		return nil, wrap(err)
	}
	return &p, nil
}

func (r *Repository) CreateProduct(ctx context.Context, p *model.Product) error {
	defer r.InvalidateCatalog()
	return wrap(r.db(ctx).Create(p).Error)
}

// UpdateProductFields точечно обновляет поля товара (без перезаписи связей).
func (r *Repository) UpdateProductFields(ctx context.Context, id uint, fields map[string]any) error {
	defer r.InvalidateCatalog()
	res := r.db(ctx).Model(&model.Product{}).Where("id = ? AND archived_at IS NULL", id).Updates(fields)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceVariants заменяет варианты товара новым набором.
// Старые не удаляются жёстко, а архивируются: на них ссылаются order_items
// прошлых заказов. Не использованные нигде варианты подчищаем физически.
func (r *Repository) ReplaceVariants(ctx context.Context, productID uint, variants []model.ProductVariant) error {
	defer r.InvalidateCatalog()
	now := r.now()
	return r.Tx(ctx, func(tx *Repository) error {
		if err := tx.db(ctx).Model(&model.ProductVariant{}).
			Where("product_id = ? AND archived_at IS NULL", productID).
			Update("archived_at", &now).Error; err != nil {
			return fmt.Errorf("архивация вариантов: %w", err)
		}
		if err := tx.db(ctx).Where(`product_id = ? AND archived_at IS NOT NULL
			AND id NOT IN (SELECT variant_id FROM order_items)`, productID).
			Delete(&model.ProductVariant{}).Error; err != nil {
			return fmt.Errorf("подчистка вариантов: %w", err)
		}
		if len(variants) == 0 {
			return nil
		}
		for i := range variants {
			variants[i].ID = 0
			variants[i].ProductID = productID
			variants[i].ArchivedAt = nil
		}
		if err := tx.db(ctx).Create(&variants).Error; err != nil {
			return fmt.Errorf("создание вариантов: %w", err)
		}
		return nil
	})
}

// ReplaceImages заменяет все фото товара.
func (r *Repository) ReplaceImages(ctx context.Context, productID uint, urls []string) error {
	defer r.InvalidateCatalog()
	return r.Tx(ctx, func(tx *Repository) error {
		if err := tx.db(ctx).Where("product_id = ?", productID).
			Delete(&model.ProductImage{}).Error; err != nil {
			return fmt.Errorf("удаление фото: %w", err)
		}
		if len(urls) == 0 {
			return nil
		}
		images := make([]model.ProductImage, 0, len(urls))
		for _, u := range urls {
			images = append(images, model.ProductImage{ProductID: productID, URL: u})
		}
		if err := tx.db(ctx).Create(&images).Error; err != nil {
			return fmt.Errorf("сохранение фото: %w", err)
		}
		return nil
	})
}

// SetProductDiscount проставляет старую цену вариантов из текущей и процента
// скидки (percent 0 — убрать). Бейдж −N% считается по old_price/price.
func (r *Repository) SetProductDiscount(ctx context.Context, id uint, percent int) error {
	defer r.InvalidateCatalog()
	if percent <= 0 || percent >= 100 {
		return wrap(r.db(ctx).Model(&model.ProductVariant{}).
			Where("product_id = ? AND archived_at IS NULL", id).
			Update("old_price", 0).Error)
	}
	// old_price = price / (1 - percent/100), одним UPDATE вместо цикла.
	return wrap(r.db(ctx).Model(&model.ProductVariant{}).
		Where("product_id = ? AND archived_at IS NULL", id).
		Update("old_price", gorm.Expr("price * 100 / ?", 100-percent)).Error)
}

// DeleteProduct — «удаление» товара через soft delete: жёсткий DELETE невозможен,
// потому что order_items ссылается на product_variants, а история заказов
// должна оставаться читаемой.
func (r *Repository) DeleteProduct(ctx context.Context, id uint) error {
	defer r.InvalidateCatalog()
	now := r.now()
	res := r.db(ctx).Model(&model.Product{}).Where("id = ? AND archived_at IS NULL", id).
		Updates(map[string]any{"is_hidden": true, "archived_at": &now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// VariantForOrder — живой вариант живого товара плюс имя и остаток товара.
// Одним запросом, чтобы не ходить в БД дважды на каждую позицию корзины.
type VariantForOrder struct {
	VariantID   uint
	ProductID   uint
	Price       int
	ProductName string
}

// GetVariantsForOrder возвращает данные по вариантам корзины.
// Архивные варианты и недоступные товары в результат не попадают —
// вызывающий код обязан сверить количество полученных строк.
func (r *Repository) GetVariantsForOrder(ctx context.Context, ids []uint) ([]VariantForOrder, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out []VariantForOrder
	err := r.db(ctx).Model(&model.ProductVariant{}).
		Select(`product_variants.id AS variant_id, product_variants.product_id,
		        product_variants.price, products.name AS product_name`).
		Joins("JOIN products ON products.id = product_variants.product_id").
		Where("product_variants.id IN ?", ids).
		Where("product_variants.archived_at IS NULL").
		Where("products.is_hidden = FALSE AND products.archived_at IS NULL").
		Scan(&out).Error
	if err != nil {
		return nil, fmt.Errorf("варианты корзины: %w", err)
	}
	return out, nil
}

// DecrementStock списывает остаток товара.
//
// stock IS NULL означает «учёт не ведётся»: NULL - qty снова даёт NULL, строка
// формально обновляется, и заказ проходит. Если остаток задан и его не хватает,
// условие WHERE не выполняется и мы получаем ноль строк → ErrOutOfStock.
// Проверку делает сама БД под блокировкой строки, поэтому два одновременных
// заказа не уведут остаток в минус.
func (r *Repository) DecrementStock(ctx context.Context, productID uint, qty int) error {
	defer r.InvalidateCatalog()
	res := r.db(ctx).Exec(`
		UPDATE products SET stock = stock - ?
		WHERE id = ? AND (stock IS NULL OR stock >= ?)`, qty, productID, qty)
	if res.Error != nil {
		return fmt.Errorf("списание остатка: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrOutOfStock
	}
	return nil
}

// RestoreStockForOrder возвращает остатки при отмене заказа: иначе отменённые
// заказы навсегда «съедали» бы товар со склада.
// Товары без учёта (stock IS NULL) не трогаются.
func (r *Repository) RestoreStockForOrder(ctx context.Context, orderID uint) error {
	defer r.InvalidateCatalog()
	err := r.db(ctx).Exec(`
		UPDATE products p
		SET stock = p.stock + s.qty
		FROM (
			SELECT v.product_id, SUM(oi.quantity) AS qty
			FROM order_items oi
			JOIN product_variants v ON v.id = oi.variant_id
			WHERE oi.order_id = ?
			GROUP BY v.product_id
		) s
		WHERE p.id = s.product_id AND p.stock IS NOT NULL`, orderID).Error
	if err != nil {
		return fmt.Errorf("возврат остатков: %w", err)
	}
	return nil
}

// ErrOutOfStock — остатка товара не хватает на заказанное количество.
var ErrOutOfStock = errors.New("недостаточно остатка")

// ─── Клиенты ───────────────────────────────────────────────────────────────

// UpsertUser находит или создаёт клиента по telegram_id и обновляет имя/телефон.
// telegram_id обязателен и уникален: «общего гостя» в системе больше нет.
func (r *Repository) UpsertUser(ctx context.Context, telegramID int64, name, phone string) (*model.User, error) {
	if telegramID == 0 {
		return nil, errors.New("telegram_id обязателен")
	}
	// ON CONFLICT вместо SELECT+INSERT: два параллельных запроса одного
	// клиента не создадут дубль и не поймают ошибку уникальности.
	err := r.db(ctx).Exec(`
		INSERT INTO users (telegram_id, name, phone, created_at)
		VALUES (?, ?, ?, NOW())
		ON CONFLICT (telegram_id) DO UPDATE
		SET name  = CASE WHEN EXCLUDED.name  <> '' THEN EXCLUDED.name  ELSE users.name  END,
		    phone = CASE WHEN EXCLUDED.phone <> '' THEN EXCLUDED.phone ELSE users.phone END`,
		telegramID, name, phone).Error
	if err != nil {
		return nil, fmt.Errorf("сохранение клиента: %w", err)
	}
	var u model.User
	if err := r.db(ctx).Where("telegram_id = ?", telegramID).First(&u).Error; err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}

func (r *Repository) GetUserByTelegramID(ctx context.Context, telegramID int64) (*model.User, error) {
	var u model.User
	err := r.db(ctx).Preload("PromoCode").Where("telegram_id = ?", telegramID).First(&u).Error
	if err != nil {
		return nil, wrap(err)
	}
	return &u, nil
}

func (r *Repository) SetUserPromo(ctx context.Context, userID uint, promoID uint) error {
	return wrap(r.db(ctx).Model(&model.User{}).Where("id = ?", userID).
		Update("promo_code_id", promoID).Error)
}

// ─── Промокоды ─────────────────────────────────────────────────────────────

func (r *Repository) GetPromoByCode(ctx context.Context, code string) (*model.PromoCode, error) {
	var p model.PromoCode
	// UPPER(code) — под уникальный функциональный индекс idx_promo_codes_upper.
	err := r.db(ctx).Where("UPPER(code) = UPPER(?)", strings.TrimSpace(code)).First(&p).Error
	if err != nil {
		return nil, wrap(err)
	}
	return &p, nil
}

func (r *Repository) GetPromoByID(ctx context.Context, id uint) (*model.PromoCode, error) {
	var p model.PromoCode
	if err := r.db(ctx).First(&p, id).Error; err != nil {
		return nil, wrap(err)
	}
	return &p, nil
}

// LockPromo берёт строку промокода под блокировку до конца транзакции.
// Нужно, чтобы два одновременных заказа не пробили max_uses.
func (r *Repository) LockPromo(ctx context.Context, id uint) (*model.PromoCode, error) {
	var p model.PromoCode
	err := r.db(ctx).Raw(`SELECT * FROM promo_codes WHERE id = ? FOR UPDATE`, id).Scan(&p).Error
	if err != nil {
		return nil, fmt.Errorf("блокировка промокода: %w", err)
	}
	if p.ID == 0 {
		return nil, ErrNotFound
	}
	return &p, nil
}

// CountUserRedemptions — сколько раз клиент уже применял этот промокод.
func (r *Repository) CountUserRedemptions(ctx context.Context, promoID, userID uint) (int, error) {
	var n int64
	err := r.db(ctx).Model(&model.PromoRedemption{}).
		Where("promo_code_id = ? AND user_id = ?", promoID, userID).
		Count(&n).Error
	return int(n), err
}

// ListPromos — все промокоды, свежие сверху. Их десятки, не тысячи:
// отдельная пагинация тут только мешала бы.
func (r *Repository) ListPromos(ctx context.Context) ([]model.PromoCode, error) {
	var out []model.PromoCode
	err := r.db(ctx).Order("is_active DESC, id DESC").Limit(100).Find(&out).Error
	return out, wrap(err)
}

func (r *Repository) CreatePromo(ctx context.Context, p *model.PromoCode) error {
	return wrap(r.db(ctx).Create(p).Error)
}

// SetPromoActive включает и выключает промокод.
func (r *Repository) SetPromoActive(ctx context.Context, id uint, active bool) error {
	res := r.db(ctx).Model(&model.PromoCode{}).Where("id = ?", id).Update("is_active", active)
	if res.Error != nil {
		return wrap(res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RedeemPromo фиксирует применение промокода. Вызывается внутри транзакции заказа.
func (r *Repository) RedeemPromo(ctx context.Context, promoID, userID, orderID uint) error {
	if err := r.db(ctx).Create(&model.PromoRedemption{
		PromoCodeID: promoID, UserID: userID, OrderID: orderID,
	}).Error; err != nil {
		return fmt.Errorf("списание промокода: %w", err)
	}
	return wrap(r.db(ctx).Model(&model.PromoCode{}).Where("id = ?", promoID).
		Update("uses", gorm.Expr("uses + 1")).Error)
}

// ─── Заказы ────────────────────────────────────────────────────────────────

func (r *Repository) CreateOrder(ctx context.Context, o *model.Order) error {
	return wrap(r.db(ctx).Create(o).Error)
}

// FindOrderByIdempotencyKey — заказ, уже созданный этим же ключом.
func (r *Repository) FindOrderByIdempotencyKey(ctx context.Context, key string) (*model.Order, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	var o model.Order
	err := r.db(ctx).Where("idempotency_key = ?", key).First(&o).Error
	if err != nil {
		return nil, wrap(err)
	}
	return &o, nil
}

func (r *Repository) GetOrder(ctx context.Context, id uint) (*model.Order, error) {
	var o model.Order
	err := r.db(ctx).Preload("User").Preload("PromoCode").
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Preload("Items.Variant").
		First(&o, id).Error
	if err != nil {
		return nil, wrap(err)
	}
	return &o, nil
}

// ListUserOrders — заказы клиента для экрана «мои заказы».
func (r *Repository) ListUserOrders(ctx context.Context, userID uint, limit int) ([]model.Order, error) {
	var orders []model.Order
	err := r.db(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB { return db.Order("id ASC") }).
		Where("user_id = ?", userID).
		Order("id DESC").Limit(limit).
		Find(&orders).Error
	return orders, wrap(err)
}

// ─── «Сегодня на базе» ─────────────────────────────────────────────────────

// UpsertFreshToday сохраняет список свежих цветов за дату (перезаписывает).
func (r *Repository) UpsertFreshToday(ctx context.Context, date, items string) error {
	return wrap(r.db(ctx).Exec(`
		INSERT INTO fresh_todays (date, items, created_at) VALUES (?, ?, NOW())
		ON CONFLICT (date) DO UPDATE SET items = EXCLUDED.items`, date, items).Error)
}

func (r *Repository) GetFreshToday(ctx context.Context, date string) (*model.FreshToday, error) {
	var f model.FreshToday
	if err := r.db(ctx).Where("date = ?", date).First(&f).Error; err != nil {
		return nil, wrap(err)
	}
	return &f, nil
}

// Stats — сводка состояния базы. Пишется в лог при старте: по ней сразу
// видно, что данные на месте, и можно сравнить состояние до и после деплоя.
type Stats struct {
	Products        int64
	VisibleProducts int64
	HiddenProducts  int64
	ArchivedProduct int64
	Variants        int64
	Images          int64
	Orders          int64
	ActiveOrders    int64
	Users           int64
	PromoCodes      int64
	Uploads         int64
}

func (r *Repository) Stats(ctx context.Context) (*Stats, error) {
	var s Stats
	err := r.db(ctx).Raw(`
		SELECT
		  (SELECT COUNT(*) FROM products)                                              AS products,
		  (SELECT COUNT(*) FROM products WHERE is_hidden = FALSE
		                                   AND archived_at IS NULL)                    AS visible_products,
		  (SELECT COUNT(*) FROM products WHERE is_hidden = TRUE)                       AS hidden_products,
		  (SELECT COUNT(*) FROM products WHERE archived_at IS NOT NULL)                AS archived_product,
		  (SELECT COUNT(*) FROM product_variants)                                      AS variants,
		  (SELECT COUNT(*) FROM product_images)                                        AS images,
		  (SELECT COUNT(*) FROM orders)                                                AS orders,
		  (SELECT COUNT(*) FROM orders WHERE status NOT IN ('delivered','cancelled'))  AS active_orders,
		  (SELECT COUNT(*) FROM users)                                                 AS users,
		  (SELECT COUNT(*) FROM promo_codes)                                           AS promo_codes,
		  (SELECT COUNT(*) FROM uploads)                                               AS uploads
	`).Scan(&s).Error
	if err != nil {
		return nil, fmt.Errorf("сводка по базе: %w", err)
	}
	return &s, nil
}

// Ping — живость соединения с БД для health-эндпоинта.
func (r *Repository) Ping(ctx context.Context) error {
	sqlDB, err := r.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// SetDailyPick назначает букет дня на дату date (YYYY-MM-DD).
// Прежний выбор на эту же дату снимается в той же транзакции: уникальный
// индекс idx_products_daily_pick иначе просто отклонил бы вставку, и админ
// получил бы ошибку вместо смены букета.
func (r *Repository) SetDailyPick(ctx context.Context, productID uint, date string) error {
	defer r.InvalidateCatalog()
	return r.Tx(ctx, func(tx *Repository) error {
		if err := tx.db(ctx).Model(&model.Product{}).
			Where("daily_pick_on = ? AND id <> ?", date, productID).
			Update("daily_pick_on", nil).Error; err != nil {
			return fmt.Errorf("снятие прежнего букета дня: %w", err)
		}
		res := tx.db(ctx).Model(&model.Product{}).
			Where("id = ? AND archived_at IS NULL", productID).
			Update("daily_pick_on", date)
		if res.Error != nil {
			return fmt.Errorf("букет дня: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ClearDailyPick снимает пометку «букет дня» с товара.
func (r *Repository) ClearDailyPick(ctx context.Context, productID uint) error {
	return r.UpdateProductFields(ctx, productID, map[string]any{"daily_pick_on": nil})
}

// SetFreshUntil помечает товар свежей поставкой до указанного момента
// (nil — снять пометку сразу, не дожидаясь срока).
func (r *Repository) SetFreshUntil(ctx context.Context, productID uint, until *time.Time) error {
	return r.UpdateProductFields(ctx, productID, map[string]any{"fresh_until": until})
}
