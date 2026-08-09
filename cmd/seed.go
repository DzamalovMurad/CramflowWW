package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

// Сидинг демо-каталога. Нужен ровно один раз — чтобы витрина не была пустой
// в первый день, пока флорист не завёл свои товары через /add.
// Фото лежат в web/public/seed/ и попадают в сборку фронтенда.

type seedProduct struct {
	name, desc, category string
	slug                 string // префикс фото в web/public/seed/
	variants             []model.ProductVariant
}

var seedProducts = []seedProduct{
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

// runSeed наполняет пустую БД демо-товарами и промокодами.
// Повторный запуск ничего не дублирует.
func runSeed(ctx context.Context, repo *repository.Repository, log *slog.Logger) error {
	existing, err := repo.ListAllProducts(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		log.Info("товары уже есть — сидинг пропущен", "count", len(existing))
		return nil
	}

	for _, sp := range seedProducts {
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
		if err := repo.CreateProduct(ctx, p); err != nil {
			return fmt.Errorf("сидинг товара %q: %w", sp.name, err)
		}
	}

	// Приветственный код: одна скидка на клиента, без срока действия.
	if err := repo.DB.WithContext(ctx).Exec(`
		INSERT INTO promo_codes (code, discount_type, discount_value, per_user_limit, is_active)
		VALUES ('WELCOME10', 'percent', 10, 1, TRUE)
		ON CONFLICT (code) DO NOTHING`).Error; err != nil {
		return fmt.Errorf("сидинг промокода: %w", err)
	}

	log.Info("демо-данные загружены", "products", len(seedProducts))
	return nil
}
