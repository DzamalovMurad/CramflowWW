package repository

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/dzamalovmurad/cramflowww/internal/migrate"
	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Интеграционные тесты soft delete. Запуск:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/repository/ -run Integration
//
// Без переменной окружения тесты пропускаются (обычный go test остаётся быстрым).
func testRepo(t *testing.T) *Repository {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционные тесты пропущены")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("подключение к тестовой БД: %v", err)
	}
	// Схему поднимаем теми же миграциями, что и прод: тест, который проверяет
	// репозиторий на схеме от AutoMigrate, ничего не сказал бы о том, работает
	// ли код на реальной базе.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("пул соединений: %v", err)
	}
	if err := migrate.Up(sqlDB); err != nil {
		t.Fatalf("миграции: %v", err)
	}
	// Чистое состояние перед каждым тестом.
	db.Exec(`TRUNCATE order_items, orders, order_status_logs, product_variants,
		product_images, products, users RESTART IDENTITY CASCADE`)
	return New(db)
}

// Товар, на вариант которого ссылается заказ, должен «удаляться» без ошибки FK
// и исчезать с витрины, а заказ — продолжать читаться.
func TestIntegrationDeleteProductWithOrders(t *testing.T) {
	r := testRepo(t)

	p := &model.Product{Name: "Розы Эквадор", Category: model.CategoryPremium,
		Variants: []model.ProductVariant{{Quantity: 9, Price: 2990}}}
	if err := r.CreateProduct(p); err != nil {
		t.Fatalf("создание товара: %v", err)
	}
	variantID := p.Variants[0].ID

	user, err := r.UpsertUser(555, "Тест", "+79000000000")
	if err != nil {
		t.Fatalf("создание клиента: %v", err)
	}
	order := &model.Order{
		UserID: user.ID, TotalPrice: 2990, DeliveryAddress: "ул. Тестовая, 1",
		DeliveryDate: "2026-08-01", DeliveryTime: "в течение часа", Status: model.StatusNew,
		Items: []model.OrderItem{{VariantID: variantID, Quantity: 1, Price: 2990, ProductName: p.Name}},
	}
	if err := r.CreateOrder(order); err != nil {
		t.Fatalf("создание заказа: %v", err)
	}

	// Ровно тот вызов, который раньше падал с SQLSTATE 23503.
	if err := r.DeleteProduct(p.ID); err != nil {
		t.Fatalf("DeleteProduct вернул ошибку: %v", err)
	}

	// Витрина товар больше не отдаёт.
	r.InvalidateCatalog()
	list, err := r.ListProducts("", "", "")
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	for _, item := range list {
		if item.ID == p.ID {
			t.Error("архивный товар всё ещё на витрине")
		}
	}

	// Заказ читается, позиция на месте.
	got, err := r.GetOrder(order.ID)
	if err != nil {
		t.Fatalf("заказ перестал читаться после удаления товара: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].ProductName != "Розы Эквадор" {
		t.Errorf("позиция заказа потерялась: %+v", got.Items)
	}
}

// Правка цен у товара, варианты которого уже в заказах, не должна падать на FK.
func TestIntegrationReplaceVariantsWithOrders(t *testing.T) {
	r := testRepo(t)

	p := &model.Product{Name: "Тюльпаны", Category: model.CategoryStandard,
		Variants: []model.ProductVariant{{Quantity: 15, Price: 2290}}}
	if err := r.CreateProduct(p); err != nil {
		t.Fatalf("создание товара: %v", err)
	}
	oldVariantID := p.Variants[0].ID

	user, _ := r.UpsertUser(556, "Тест2", "+79000000001")
	order := &model.Order{
		UserID: user.ID, TotalPrice: 2290, DeliveryAddress: "ул. Тестовая, 2",
		DeliveryDate: "2026-08-02", DeliveryTime: "в течение часа", Status: model.StatusNew,
		Items: []model.OrderItem{{VariantID: oldVariantID, Quantity: 1, Price: 2290, ProductName: p.Name}},
	}
	if err := r.CreateOrder(order); err != nil {
		t.Fatalf("создание заказа: %v", err)
	}

	// Админ меняет цены — раньше здесь тоже был FK-конфликт.
	newVariants := []model.ProductVariant{{Quantity: 15, Price: 2490}, {Quantity: 25, Price: 3690}}
	if err := r.ReplaceVariants(p.ID, newVariants); err != nil {
		t.Fatalf("ReplaceVariants вернул ошибку: %v", err)
	}

	// Витрина показывает только новые варианты.
	fresh, err := r.GetProduct(p.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if len(fresh.Variants) != 2 {
		t.Fatalf("ожидали 2 живых варианта, получили %d", len(fresh.Variants))
	}
	for _, v := range fresh.Variants {
		if v.ID == oldVariantID {
			t.Error("архивный вариант попал на витрину")
		}
		if v.Price != 2490 && v.Price != 3690 {
			t.Errorf("неожиданная цена варианта: %d", v.Price)
		}
	}

	// Старый вариант больше не заказать.
	if _, err := r.GetVariant(oldVariantID); err == nil {
		t.Error("архивный вариант всё ещё доступен для заказа")
	}

	// Старый заказ по-прежнему читается.
	if _, err := r.GetOrder(order.ID); err != nil {
		t.Fatalf("заказ перестал читаться после правки цен: %v", err)
	}
}
