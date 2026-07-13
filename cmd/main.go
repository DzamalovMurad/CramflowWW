package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/dzamalovmurad/cramflowww/internal/handler"
	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

func main() {
	seed := flag.Bool("seed", false, "заполнить БД тестовыми данными и выйти")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL не задан")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("подключение к БД: %v", err)
	}

	// Миграции: GORM AutoMigrate покрывает всю схему (SQL-эквивалент — в /migrations).
	if err := db.AutoMigrate(
		&model.Product{}, &model.ProductVariant{}, &model.ProductImage{},
		&model.PromoCode{}, &model.User{}, &model.Order{}, &model.OrderItem{},
		&model.FreshToday{},
	); err != nil {
		log.Fatalf("миграции: %v", err)
	}

	uploadDir := envOr("UPLOAD_DIR", "./uploads")
	store, err := storage.NewLocal(uploadDir, "/uploads")
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	repo := repository.New(db)
	svc := service.New(repo)

	if *seed {
		if err := runSeed(repo, uploadDir); err != nil {
			log.Fatalf("сидинг: %v", err)
		}
		log.Println("тестовые данные загружены")
		return
	}

	// Бот опционален: без BOT_TOKEN сервис работает как чистый API (удобно для разработки).
	botToken := os.Getenv("BOT_TOKEN")
	if botToken != "" {
		adminChatID, _ := strconv.ParseInt(os.Getenv("ADMIN_CHAT_ID"), 10, 64)
		if adminChatID == 0 {
			log.Println("внимание: ADMIN_CHAT_ID не задан — админ-команды будут недоступны")
		}
		bot, err := handler.NewBot(botToken, adminChatID, os.Getenv("TELEGRAM_APP_URL"), repo, svc, store)
		if err != nil {
			log.Fatalf("бот: %v", err)
		}
		go bot.Run()
	} else {
		log.Println("BOT_TOKEN не задан — запуск без бота")
	}

	api := &handler.API{
		Repo:      repo,
		Service:   svc,
		BotToken:  botToken,
		UploadDir: uploadDir,
		WebDist:   envOr("WEB_DIST", "./web/dist"),
	}

	port := envOr("PORT", "8080")
	log.Printf("HTTP-сервер на :%s", port)
	if err := http.ListenAndServe(":"+port, api.Routes()); err != nil {
		log.Fatal(err)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

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
