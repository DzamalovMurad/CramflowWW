package repository_test

import (
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/testdb"
)

func testRepo(t *testing.T) *repository.Repository {
	t.Helper()
	return repository.New(testdb.Open(t), time.Now)
}

func mustProduct(t *testing.T, r *repository.Repository, name, category string, price int) *model.Product {
	t.Helper()
	p := &model.Product{
		Name: name, Category: category,
		Variants: []model.ProductVariant{{Quantity: 9, Price: price}},
		Images:   []model.ProductImage{{URL: "/uploads/1.jpg"}},
	}
	if err := r.CreateProduct(t.Context(), p); err != nil {
		t.Fatalf("создание товара %q: %v", name, err)
	}
	return p
}

// Товар, на вариант которого ссылается заказ, должен «удаляться» без ошибки FK
// и исчезать с витрины, а заказ — продолжать читаться.
func TestIntegrationDeleteProductWithOrders(t *testing.T) {
	r := testRepo(t)
	ctx := t.Context()

	p := mustProduct(t, r, "Розы Эквадор", model.CategoryPremium, 2990)
	user, err := r.UpsertUser(ctx, 555, "Тест", "+79000000000")
	if err != nil {
		t.Fatalf("создание клиента: %v", err)
	}
	order := &model.Order{
		UserID: user.ID, SubtotalPrice: 2990, TotalPrice: 2990,
		DeliveryAddress: "ул. Тестовая, 1", DeliveryDate: "2026-08-01",
		DeliveryTime: "в течение часа", Status: model.StatusNew,
		Items: []model.OrderItem{{
			VariantID: p.Variants[0].ID, Quantity: 1, Price: 2990, ProductName: p.Name,
		}},
	}
	if err := r.CreateOrder(ctx, order); err != nil {
		t.Fatalf("создание заказа: %v", err)
	}

	// Ровно тот вызов, который раньше падал с SQLSTATE 23503.
	if err := r.DeleteProduct(ctx, p.ID); err != nil {
		t.Fatalf("DeleteProduct вернул ошибку: %v", err)
	}

	list, err := r.ListProducts(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	for _, item := range list {
		if item.ID == p.ID {
			t.Error("архивный товар всё ещё на витрине")
		}
	}
	// И в админском списке его тоже нет.
	all, err := r.ListAllProducts(ctx)
	if err != nil {
		t.Fatalf("ListAllProducts: %v", err)
	}
	for _, item := range all {
		if item.ID == p.ID {
			t.Error("архивный товар остался в админском списке")
		}
	}
	// Прямое чтение архивного товара тоже недоступно.
	if _, err := r.GetProduct(ctx, p.ID); err == nil {
		t.Error("архивный товар не должен читаться через GetProduct")
	}

	got, err := r.GetOrder(ctx, order.ID)
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
	ctx := t.Context()

	p := mustProduct(t, r, "Тюльпаны", model.CategoryStandard, 2290)
	oldVariantID := p.Variants[0].ID

	user, _ := r.UpsertUser(ctx, 556, "Тест2", "+79000000001")
	order := &model.Order{
		UserID: user.ID, SubtotalPrice: 2290, TotalPrice: 2290,
		DeliveryAddress: "ул. Тестовая, 2", DeliveryDate: "2026-08-02",
		DeliveryTime: "в течение часа", Status: model.StatusNew,
		Items: []model.OrderItem{{
			VariantID: oldVariantID, Quantity: 1, Price: 2290, ProductName: p.Name,
		}},
	}
	if err := r.CreateOrder(ctx, order); err != nil {
		t.Fatalf("создание заказа: %v", err)
	}

	newVariants := []model.ProductVariant{{Quantity: 15, Price: 2490}, {Quantity: 25, Price: 3690}}
	if err := r.ReplaceVariants(ctx, p.ID, newVariants); err != nil {
		t.Fatalf("ReplaceVariants вернул ошибку: %v", err)
	}

	fresh, err := r.GetProduct(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if len(fresh.Variants) != 2 {
		t.Fatalf("ожидали 2 живых варианта, получили %d", len(fresh.Variants))
	}
	// Варианты отсортированы по цене — от этого зависит цена «от» на витрине.
	if fresh.Variants[0].Price != 2490 || fresh.Variants[1].Price != 3690 {
		t.Errorf("варианты не отсортированы по цене: %+v", fresh.Variants)
	}
	for _, v := range fresh.Variants {
		if v.ID == oldVariantID {
			t.Error("архивный вариант попал на витрину")
		}
	}

	// Старый вариант больше не заказать.
	rows, err := r.GetVariantsForOrder(ctx, []uint{oldVariantID})
	if err != nil {
		t.Fatalf("GetVariantsForOrder: %v", err)
	}
	if len(rows) != 0 {
		t.Error("архивный вариант всё ещё доступен для заказа")
	}

	if _, err := r.GetOrder(ctx, order.ID); err != nil {
		t.Fatalf("заказ перестал читаться после правки цен: %v", err)
	}
}

// Скрытый товар исчезает с витрины, но остаётся в админском списке.
func TestIntegrationHiddenProductVisibility(t *testing.T) {
	r := testRepo(t)
	ctx := t.Context()

	p := mustProduct(t, r, "Скрываемый букет", model.CategoryStandard, 1000)
	if err := r.UpdateProductFields(ctx, p.ID, map[string]any{"is_hidden": true}); err != nil {
		t.Fatalf("скрытие: %v", err)
	}

	list, _ := r.ListProducts(ctx, "", "", "")
	for _, item := range list {
		if item.ID == p.ID {
			t.Error("скрытый товар на витрине")
		}
	}
	all, _ := r.ListAllProducts(ctx)
	found := false
	for _, item := range all {
		if item.ID == p.ID {
			found = true
		}
	}
	if !found {
		t.Error("скрытый товар пропал из админского списка")
	}
}

// Кэш витрины обязан сбрасываться при правках: иначе админ меняет цену,
// а клиент минуту видит старую.
func TestIntegrationCatalogCacheInvalidation(t *testing.T) {
	r := testRepo(t)
	ctx := t.Context()

	p := mustProduct(t, r, "Кэшируемый", model.CategoryStandard, 1000)
	if _, err := r.ListProducts(ctx, "", "", ""); err != nil { // прогрев кэша
		t.Fatalf("ListProducts: %v", err)
	}

	if err := r.ReplaceVariants(ctx, p.ID, []model.ProductVariant{{Quantity: 9, Price: 7777}}); err != nil {
		t.Fatalf("ReplaceVariants: %v", err)
	}
	list, err := r.ListProducts(ctx, "", "", "")
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	for _, item := range list {
		if item.ID == p.ID {
			if len(item.Variants) == 0 || item.Variants[0].Price != 7777 {
				t.Errorf("витрина отдала старую цену: %+v", item.Variants)
			}
			return
		}
	}
	t.Error("товар пропал с витрины после правки цен")
}

// Фильтры и поиск должны отдавать ровно то, что обещают.
func TestIntegrationFiltersAndSearch(t *testing.T) {
	r := testRepo(t)
	ctx := t.Context()

	mustProduct(t, r, "Розы Эквадор", model.CategoryPremium, 5000)
	mustProduct(t, r, "Хризантемы кустовые", model.CategoryStandard, 1500)
	mustProduct(t, r, "Тюльпаны Пинк", model.CategoryStandard, 2900)

	cases := []struct {
		name             string
		category, filter string
		search           string
		wantNames        []string
	}{
		{"всё", "", "", "", []string{"Розы Эквадор", "Хризантемы кустовые", "Тюльпаны Пинк"}},
		{"категория", model.CategoryPremium, "", "", []string{"Розы Эквадор"}},
		{"бюджет", "", repository.FilterBudget, "", []string{"Хризантемы кустовые", "Тюльпаны Пинк"}},
		{"новинки", "", repository.FilterNew, "", []string{"Розы Эквадор", "Хризантемы кустовые", "Тюльпаны Пинк"}},
		{"поиск с опечаткой", "", "", "хрезантемы", []string{"Хризантемы кустовые"}},
		{"поиск по корню", "", "", "роза", []string{"Розы Эквадор"}},
		{"поиск мимо", "", "", "кактус", nil},
		{"популярное без продаж", "", repository.FilterPopular, "", []string{"Розы Эквадор", "Хризантемы кустовые", "Тюльпаны Пинк"}},
	}

	for _, c := range cases {
		got, err := r.ListProducts(ctx, c.category, c.filter, c.search)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		names := map[string]bool{}
		for _, p := range got {
			names[p.Name] = true
		}
		if len(got) != len(c.wantNames) {
			t.Errorf("%s: получили %d товаров, ожидали %d (%v)", c.name, len(got), len(c.wantNames), names)
			continue
		}
		for _, want := range c.wantNames {
			if !names[want] {
				t.Errorf("%s: в результате нет %q", c.name, want)
			}
		}
	}
}

// «Популярное» должно ставить наверх то, что реально покупают.
func TestIntegrationPopularOrdering(t *testing.T) {
	r := testRepo(t)
	ctx := t.Context()

	quiet := mustProduct(t, r, "Непопулярный", model.CategoryStandard, 1000)
	hot := mustProduct(t, r, "Хит продаж", model.CategoryStandard, 1000)

	user, _ := r.UpsertUser(ctx, 900, "Покупатель", "+79000000009")
	for i := 0; i < 3; i++ {
		o := &model.Order{
			UserID: user.ID, SubtotalPrice: 1000, TotalPrice: 1000,
			DeliveryAddress: "адрес", DeliveryDate: "2026-08-01",
			DeliveryTime: "в течение часа", Status: model.StatusNew,
			Items: []model.OrderItem{{
				VariantID: hot.Variants[0].ID, Quantity: 5, Price: 1000, ProductName: hot.Name,
			}},
		}
		if err := r.CreateOrder(ctx, o); err != nil {
			t.Fatalf("заказ: %v", err)
		}
	}
	_ = quiet

	got, err := r.ListProducts(ctx, "", repository.FilterPopular, "")
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("ожидали 2 товара, получили %d", len(got))
	}
	if got[0].Name != "Хит продаж" {
		t.Errorf("на первом месте %q, ожидали «Хит продаж»", got[0].Name)
	}
}

// UpsertUser не должен создавать дублей при одновременных заказах клиента.
func TestIntegrationUpsertUserIsIdempotent(t *testing.T) {
	r := testRepo(t)
	ctx := t.Context()

	first, err := r.UpsertUser(ctx, 12345, "Иван", "+79001112233")
	if err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	// Повтор без имени/телефона не должен затирать сохранённое.
	second, err := r.UpsertUser(ctx, 12345, "", "")
	if err != nil {
		t.Fatalf("UpsertUser повтор: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("создан дубль клиента: #%d и #%d", first.ID, second.ID)
	}
	if second.Name != "Иван" || second.Phone != "+79001112233" {
		t.Errorf("пустые значения затёрли контакты: %q / %q", second.Name, second.Phone)
	}
	// А новые данные — обновляют.
	third, _ := r.UpsertUser(ctx, 12345, "Иван Петров", "+79004445566")
	if third.Name != "Иван Петров" || third.Phone != "+79004445566" {
		t.Errorf("контакты не обновились: %q / %q", third.Name, third.Phone)
	}
}

// Поиск клиентов не должен ломаться на спецсимволах шаблона LIKE.
func TestIntegrationSearchClientsEscapesPattern(t *testing.T) {
	r := testRepo(t)
	ctx := t.Context()

	if _, err := r.UpsertUser(ctx, 1, "Мурад", "+79001112233"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if _, err := r.UpsertUser(ctx, 2, "Анна", "+79004445566"); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}

	found, err := r.SearchClients(ctx, "мурад", 5)
	if err != nil {
		t.Fatalf("SearchClients: %v", err)
	}
	if len(found) != 1 || found[0].Name != "Мурад" {
		t.Errorf("поиск по имени вернул %d результатов", len(found))
	}

	// «%» не должен превращаться в «выбрать всех».
	all, err := r.SearchClients(ctx, "%", 5)
	if err != nil {
		t.Fatalf("SearchClients с %%: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("шаблон «%%» вернул %d клиентов вместо 0 — экранирование не работает", len(all))
	}
}
