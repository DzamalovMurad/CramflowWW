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

	logSchema(db)
	addMissingNotNullColumns(db)
	backfillNullDefaults(db)

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

// backfillNullDefaults заполняет NULL-ы в колонках, которые модель объявляет
// NOT NULL DEFAULT. Такие колонки добавлялись к уже существующим таблицам:
// ALTER ADD COLUMN оставил старым строкам NULL, и следующий AutoMigrate падает
// на «ALTER COLUMN … SET NOT NULL … contains null values» — приложение не стартует.
// Значения берём ровно те же, что в тегах gorm, поэтому запись идемпотентна и
// не меняет смысл данных: NULL и дефолт трактуются кодом одинаково.
// Список константный (не из пользовательского ввода) — подстановка в SQL безопасна.
func backfillNullDefaults(db *gorm.DB) {
	columns := []struct{ table, column, value string }{
		{"products", "is_hidden", "false"},
		{"products", "is_hit", "false"},
		{"products", "stock", "0"},
		{"product_variants", "old_price", "0"},
		{"orders", "status", "'new'"},
		{"promo_codes", "uses", "0"},
	}
	for _, c := range columns {
		var nullable string
		err := db.Raw(
			`SELECT is_nullable FROM information_schema.columns
			 WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
			c.table, c.column,
		).Scan(&nullable).Error
		// Колонки ещё нет (чистая БД) или она уже NOT NULL — делать нечего.
		if err != nil || nullable != "YES" {
			continue
		}
		res := db.Exec(fmt.Sprintf("UPDATE %s SET %s = %s WHERE %s IS NULL", c.table, c.column, c.value, c.column))
		if res.Error != nil {
			log.Printf("бэкфилл %s.%s: %v", c.table, c.column, res.Error)
		} else if res.RowsAffected > 0 {
			log.Printf("бэкфилл %s.%s: заполнено строк — %d", c.table, c.column, res.RowsAffected)
		}
	}
}

// addMissingNotNullColumns добавляет колонки, которые модель объявляет NOT NULL,
// а в существующей таблице их ещё нет. Сам AutoMigrate такую колонку добавить
// не может: «ADD COLUMN … NOT NULL» без DEFAULT падает на непустой таблице
// (SQLSTATE 23502), и приложение не стартует. Добавляем с DEFAULT, чтобы старые
// строки получили осмысленное значение; copyFrom переносит данные из колонки
// прежнего имени, если схема БД отстала от модели.
// Имена и типы константны (не из пользовательского ввода) — подстановка безопасна.
func addMissingNotNullColumns(db *gorm.DB) {
	columns := []struct{ table, column, ddl, copyFrom string }{
		{"products", "name", "text NOT NULL DEFAULT ''", ""},
		{"products", "category", "text NOT NULL DEFAULT ''", ""},
		{"products", "is_hidden", "boolean NOT NULL DEFAULT false", ""},
		{"products", "is_hit", "boolean NOT NULL DEFAULT false", ""},
		{"products", "stock", "bigint NOT NULL DEFAULT 0", ""},
		{"product_variants", "quantity", "bigint NOT NULL DEFAULT 0", ""},
		{"product_variants", "price", "bigint NOT NULL DEFAULT 0", ""},
		{"product_variants", "old_price", "bigint NOT NULL DEFAULT 0", ""},
		{"product_images", "url", "text NOT NULL DEFAULT ''", ""},
		{"promo_codes", "code", "text NOT NULL DEFAULT ''", ""},
		{"promo_codes", "discount_percent", "bigint NOT NULL DEFAULT 0", "discount"},
		{"promo_codes", "uses", "bigint NOT NULL DEFAULT 0", ""},
		{"orders", "total_price", "bigint NOT NULL DEFAULT 0", ""},
		{"orders", "delivery_address", "text NOT NULL DEFAULT ''", ""},
		{"orders", "delivery_date", "text NOT NULL DEFAULT ''", ""},
		{"orders", "delivery_time", "text NOT NULL DEFAULT ''", ""},
		{"orders", "status", "text NOT NULL DEFAULT 'new'", ""},
		{"order_items", "quantity", "bigint NOT NULL DEFAULT 0", ""},
		{"order_items", "price", "bigint NOT NULL DEFAULT 0", ""},
		{"order_status_logs", "from_status", "text NOT NULL DEFAULT ''", ""},
		{"order_status_logs", "to_status", "text NOT NULL DEFAULT ''", ""},
		{"order_status_logs", "admin_id", "bigint NOT NULL DEFAULT 0", ""},
		{"uploads", "ext", "text NOT NULL DEFAULT ''", ""},
		{"uploads", "mime_type", "text NOT NULL DEFAULT ''", ""},
		{"fresh_todays", "date", "text NOT NULL DEFAULT ''", ""},
		{"fresh_todays", "items", "text NOT NULL DEFAULT ''", ""},
	}
	for _, c := range columns {
		if !tableExists(db, c.table) || columnExists(db, c.table, c.column) {
			continue
		}
		if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", c.table, c.column, c.ddl)).Error; err != nil {
			log.Printf("добавление %s.%s: %v", c.table, c.column, err)
			continue
		}
		log.Printf("добавлена недостающая колонка %s.%s (%s)", c.table, c.column, c.ddl)
		if c.copyFrom != "" && columnExists(db, c.table, c.copyFrom) {
			res := db.Exec(fmt.Sprintf("UPDATE %s SET %s = %s", c.table, c.column, c.copyFrom))
			if res.Error != nil {
				log.Printf("перенос %s.%s ← %s: %v", c.table, c.column, c.copyFrom, res.Error)
			} else {
				log.Printf("перенос %s.%s ← %s: строк — %d", c.table, c.column, c.copyFrom, res.RowsAffected)
			}
		}
	}
}

func tableExists(db *gorm.DB, table string) bool {
	var reg *string
	if err := db.Raw("SELECT to_regclass(?)::text", table).Scan(&reg).Error; err != nil {
		return false
	}
	return reg != nil
}

func columnExists(db *gorm.DB, table, column string) bool {
	var name string
	err := db.Raw(
		`SELECT column_name FROM information_schema.columns
		 WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`,
		table, column,
	).Scan(&name).Error
	return err == nil && name != ""
}

// logSchema печатает фактические колонки таблиц — без этого причину отказа
// миграции видно только по одной колонке за перезапуск.
func logSchema(db *gorm.DB) {
	type row struct {
		TableName string
		Columns   string
	}
	var rows []row
	err := db.Raw(
		`SELECT table_name, string_agg(column_name || ':' || data_type ||
		        CASE WHEN is_nullable = 'YES' THEN '?' ELSE '' END, ', ' ORDER BY ordinal_position) AS columns
		 FROM information_schema.columns
		 WHERE table_schema = current_schema()
		 GROUP BY table_name ORDER BY table_name`,
	).Scan(&rows).Error
	if err != nil {
		log.Printf("схема: %v", err)
		return
	}
	for _, r := range rows {
		log.Printf("схема %s: %s", r.TableName, r.Columns)
	}
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
