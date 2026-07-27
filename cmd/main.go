package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

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

	// Пул соединений: держим мало и закрываем простаивающие.
	// На serverless-Postgres (Neon) это позволяет базе засыпать в простое —
	// иначе бесплатные CU-часы сгорают на пустых соединениях.
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(8)
		sqlDB.SetMaxIdleConns(2)
		sqlDB.SetConnMaxIdleTime(time.Minute)
		sqlDB.SetConnMaxLifetime(30 * time.Minute)
	}

	// Миграции: GORM AutoMigrate покрывает всю схему (SQL-эквивалент — в /migrations).
	if err := db.AutoMigrate(
		&model.Product{}, &model.ProductVariant{}, &model.ProductImage{},
		&model.PromoCode{}, &model.User{}, &model.Order{}, &model.OrderItem{},
		&model.FreshToday{}, &model.OrderStatusLog{},
	); err != nil {
		log.Fatalf("миграции: %v", err)
	}

	// Куда складывать фото товаров:
	//   UPLOAD_STORE=db   — в Postgres (хостинг без постоянного диска),
	//   иначе             — в папку UPLOAD_DIR (Railway Volume и локальная разработка).
	uploadDir := envOr("UPLOAD_DIR", "./uploads")
	var store storage.Storage
	var dbUploads *storage.Postgres
	if envOr("UPLOAD_STORE", "local") == "db" {
		dbUploads, err = storage.NewPostgres(db, "/uploads")
		if err != nil {
			log.Fatalf("storage: %v", err)
		}
		store = dbUploads
		log.Println("фото товаров хранятся в БД (UPLOAD_STORE=db)")
	} else {
		store, err = storage.NewLocal(uploadDir, "/uploads")
		if err != nil {
			log.Fatalf("storage: %v", err)
		}
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

	api := &handler.API{
		Repo:      repo,
		Service:   svc,
		BotToken:  os.Getenv("BOT_TOKEN"),
		UploadDir: uploadDir,
		Uploads:   dbUploads, // nil при локальном хранении — фото отдаёт FileServer
		WebDist:   envOr("WEB_DIST", "./web/dist"),
	}

	// Бот опционален: без BOT_TOKEN сервис работает как чистый API (удобно для разработки).
	if botToken := os.Getenv("BOT_TOKEN"); botToken != "" {
		adminIDs := parseAdminIDs()
		if len(adminIDs) == 0 {
			log.Println("внимание: ADMIN_IDS/ADMIN_CHAT_ID не заданы — админ-команды будут недоступны")
		}
		appURL := os.Getenv("TELEGRAM_APP_URL")
		bot, err := handler.NewBot(botToken, adminIDs, appURL, repo, svc, store)
		if err != nil {
			log.Fatalf("бот: %v", err)
		}

		// BOT_MODE=webhook — для хостингов, засыпающих без трафика: входящий
		// запрос от Telegram сам будит сервис. По умолчанию — long polling.
		if envOr("BOT_MODE", "polling") == "webhook" {
			secret := os.Getenv("WEBHOOK_SECRET")
			if secret == "" {
				log.Fatal("BOT_MODE=webhook требует WEBHOOK_SECRET")
			}
			api.WebhookPath = bot.WebhookPath(secret)
			api.WebhookHandler = bot.WebhookHandler(secret)
			if err := bot.SetupWebhook(envOr("PUBLIC_URL", appURL), secret); err != nil {
				log.Fatalf("webhook: %v", err)
			}
		} else {
			if err := bot.RemoveWebhook(); err != nil {
				log.Printf("снятие webhook: %v", err)
			}
			go bot.Run()
		}
	} else {
		log.Println("BOT_TOKEN не задан — запуск без бота")
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

// parseAdminIDs — whitelist админов: ADMIN_IDS="123,456" (приоритет)
// или одиночный ADMIN_CHAT_ID (обратная совместимость).
func parseAdminIDs() []int64 {
	var ids []int64
	for _, part := range strings.Split(os.Getenv("ADMIN_IDS"), ",") {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id != 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		if id, _ := strconv.ParseInt(os.Getenv("ADMIN_CHAT_ID"), 10, 64); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
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
