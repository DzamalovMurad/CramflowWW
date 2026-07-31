package main

import (
	"fmt"
	"log"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

// --- Сидинг тестовых данных ---

type seedProduct struct {
	name, desc, category string
	slug                 string // префикс фото в web/public/seed/
	variants             []model.ProductVariant
}

// runSeed наполняет пустую БД демо-товарами и промокодами.
// Фото лежат в web/public/seed/ и попадают в сборку фронтенда.
func runSeed(repo *repository.Repository, _ string) error {
	var count int64
	repo.DB.Model(&model.Product{}).Count(&count)
	if count > 0 {
		log.Println("товары уже есть — сидинг пропущен")
		return nil
	}

	products := []seedProduct{
		{"Розы Эквадор", "Крупные эквадорские розы глубокого красного оттенка. Стойкость до 14 дней.", model.CategoryPremium, "roses-red",
			[]model.ProductVariant{{Quantity: 9, Price: 2990}, {Quantity: 15, Price: 4490}, {Quantity: 25, Price: 6990}}},
		{"Пионовидный микс", "Пионовидные розы пастельных оттенков — нежность в каждом лепестке.", model.CategoryLux, "peony-pastel",
			[]model.ProductVariant{{Quantity: 7, Price: 4990}, {Quantity: 11, Price: 7290}}},
		{"Солнечное настроение", "Яркий жёлтый микс — маленькое солнце в вашем доме.", model.CategoryStandard, "sunny",
			[]model.ProductVariant{{Quantity: 15, Price: 1990}, {Quantity: 25, Price: 2890}}},
		{"Розовые каллы", "Элегантные каллы с розовым градиентом. Для ценителей строгих линий.", model.CategoryPremium, "calla",
			[]model.ProductVariant{{Quantity: 9, Price: 3490}, {Quantity: 13, Price: 4990}}},
		{"Тюльпаны Пинк", "Пионовидные розовые тюльпаны в лаконичной подаче.", model.CategoryStandard, "tulips-pink",
			[]model.ProductVariant{{Quantity: 15, Price: 2290}, {Quantity: 25, Price: 3390}}},
		{"Сердце из цветов", "Композиция-сердце из сезонных цветов. Признание без слов.", model.CategoryWow, "heart",
			[]model.ProductVariant{{Quantity: 25, Price: 8990}, {Quantity: 51, Price: 14990}}},
		{"Авторский гранд-букет", "Фирменный букет флориста: розы, эвкалипт, ягодники. Впечатление гарантировано.", model.CategoryWow, "wow-mix",
			[]model.ProductVariant{{Quantity: 51, Price: 12990}, {Quantity: 101, Price: 18990}}},
	}

	for _, sp := range products {
		p := &model.Product{
			Name:        sp.name,
			Description: sp.desc,
			Category:    sp.category,
			Variants:    sp.variants,
		}
		for j := 1; j <= 4; j++ {
			p.Images = append(p.Images, model.ProductImage{
				URL: fmt.Sprintf("/seed/%s-%d.webp", sp.slug, j),
			})
		}
		if err := repo.CreateProduct(p); err != nil {
			return err
		}
	}

	promos := []model.PromoCode{
		{Code: "WELCOME10", DiscountPercent: 10},
		{Code: "FLOWERS15", DiscountPercent: 15},
	}
	for i := range promos {
		if err := repo.DB.Create(&promos[i]).Error; err != nil {
			return err
		}
	}
	return nil
}
