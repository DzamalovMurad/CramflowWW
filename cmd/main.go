package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/dzamalovmurad/cramflowww/internal/backup"
	"github.com/dzamalovmurad/cramflowww/internal/handler"
	"github.com/dzamalovmurad/cramflowww/internal/migrate"
	"github.com/dzamalovmurad/cramflowww/internal/observability"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

// shutdownTimeout — сколько ждём завершения запросов и отправок после SIGTERM.
// Railway даёт контейнеру ~30 секунд, поэтому укладываемся с запасом.
const shutdownTimeout = 20 * time.Second

func main() {
	seed := flag.Bool("seed", false, "заполнить БД тестовыми данными и выйти")
	migrateOnly := flag.Bool("migrate", false, "применить миграции и выйти")
	migrateStatus := flag.Bool("migrate-status", false, "показать состояние миграций и выйти")
	migrateDown := flag.Bool("migrate-down", false, "откатить последнюю миграцию и выйти")
	flag.Parse()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL не задан")
	}

	observability.Init(os.Getenv("SENTRY_DSN"), envOr("APP_ENV", "production"), os.Getenv("RAILWAY_GIT_COMMIT_SHA"))
	defer observability.Flush(3 * time.Second)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("подключение к БД: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("пул соединений: %v", err)
	}
	// Пул соединений: держим мало и закрываем простаивающие.
	// На serverless-Postgres (Neon) это позволяет базе засыпать в простое —
	// иначе бесплатные CU-часы сгорают на пустых соединениях.
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxIdleTime(time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	// Ручные операции с миграциями (railway run ./cramflow -migrate-status и т.п.).
	switch {
	case *migrateStatus:
		if err := migrate.Status(sqlDB); err != nil {
			log.Fatalf("миграции: %v", err)
		}
		return
	case *migrateDown:
		if err := migrate.Down(sqlDB); err != nil {
			log.Fatalf("миграции: %v", err)
		}
		log.Println("последняя миграция откачена")
		return
	}

	// Схема приводится в порядок до старта сервиса: AutoMigrate в проде больше
	// нет, единственный источник правды — SQL-миграции в /migrations.
	if err := migrate.Up(sqlDB); err != nil {
		log.Fatalf("миграции: %v", err)
	}
	if v, err := migrate.Version(sqlDB); err == nil {
		log.Printf("схема БД: версия %d", v)
	}
	if *migrateOnly {
		return
	}

	// Куда складывать фото товаров:
	//   UPLOAD_STORE=db   — в Postgres (хостинг без постоянного диска),
	//   иначе             — в папку UPLOAD_DIR (Railway Volume и локальная разработка).
	uploadDir := envOr("UPLOAD_DIR", "./uploads")
	var store storage.Storage
	var dbUploads *storage.Postgres
	if envOr("UPLOAD_STORE", "local") == "db" {
		dbUploads = storage.NewPostgres(db, "/uploads")
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

	appURL := os.Getenv("TELEGRAM_APP_URL")
	api := &handler.API{
		Repo:      repo,
		Service:   svc,
		BotToken:  os.Getenv("BOT_TOKEN"),
		UploadDir: uploadDir,
		Uploads:   dbUploads, // nil при локальном хранении — фото отдаёт FileServer
		WebDist:   envOr("WEB_DIST", "./web/dist"),
		AppURL:    appURL,
	}

	var bot *handler.Bot
	var cronRunner *cron.Cron

	// Бот опционален: без BOT_TOKEN сервис работает как чистый API (удобно для разработки).
	if botToken := os.Getenv("BOT_TOKEN"); botToken != "" {
		adminIDs := parseAdminIDs()
		if len(adminIDs) == 0 {
			log.Println("внимание: ADMIN_IDS/ADMIN_CHAT_ID не заданы — админ-команды будут недоступны")
		}
		bot, err = handler.NewBot(botToken, adminIDs, appURL, repo, svc, store)
		if err != nil {
			log.Fatalf("бот: %v", err)
		}
		api.BotUsername = bot.Username()

		if appURL == "" {
			log.Println("TELEGRAM_APP_URL не задан — заказы принимаются диалогом в боте (/order)")
		}

		// Бэкапы: ночной крон + ручной /backup.
		if bs := backup.New(dsn, envOr("BACKUP_DIR", "./backups"),
			parseInt64(os.Getenv("BACKUP_CHANNEL_ID")), bot.API(), adminIDs); bs != nil {
			bot.OnBackup = func(chatID int64) {
				bs.RunManual(context.Background(), chatID)
			}
			cronRunner = cron.New()
			spec := envOr("BACKUP_CRON", "0 3 * * *") // 03:00 UTC ежедневно
			if _, err := cronRunner.AddFunc(spec, func() {
				bs.RunNightly(context.Background())
			}); err != nil {
				log.Fatalf("расписание бэкапов (BACKUP_CRON=%q): %v", spec, err)
			}
			cronRunner.Start()
			log.Printf("бэкапы включены: расписание %q, канал %s", spec, os.Getenv("BACKUP_CHANNEL_ID"))
		} else {
			log.Println("BACKUP_CHANNEL_ID не задан — автобэкапы выключены")
		}

		// Рассылка, прерванная остановкой сервиса, продолжается с того места,
		// где оборвалась, — и не пишет повторно тем, кому уже написали.
		bot.ResumeBroadcasts()

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

	api.Health = &health{db: sqlDB, bot: bot}

	port := envOr("PORT", "8080")
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: api.Routes(),
		// Таймауты нужны, чтобы зависший клиент не занимал соединение вечно.
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second, // с запасом на отдачу фото
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		log.Printf("HTTP-сервер на :%s", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP-сервер: %v", err)
		}
	}()

	// --- Graceful shutdown ---
	//
	// Railway шлёт SIGTERM и через ~30 секунд убивает контейнер. За это время
	// нужно: перестать брать новые запросы, дать доработать текущим (клиент
	// в момент деплоя не должен увидеть обрыв на оформлении заказа) и дождаться,
	// пока уйдут начатые отправки бота.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	sig := <-stop
	log.Printf("получен %s — завершаем работу", sig)

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if cronRunner != nil {
		// Ждём, пока закончится начатый дамп: оборванный pg_dump оставил бы
		// в канале битый архив.
		<-cronRunner.Stop().Done()
	}
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("HTTP-сервер: %v", err)
	} else {
		log.Println("HTTP-сервер: все запросы завершены")
	}
	if bot != nil {
		bot.Stop(ctx)
	}
	if err := sqlDB.Close(); err != nil {
		log.Printf("закрытие БД: %v", err)
	}
	observability.Flush(3 * time.Second)
	log.Println("остановлено")
}

// health — зависимости для /healthz.
type health struct {
	db  *sql.DB
	bot *handler.Bot
}

func (h *health) PingDB(ctx context.Context) error {
	return h.db.PingContext(ctx)
}

// PingBot — getMe: проверяем, что токен жив и Telegram отвечает.
func (h *health) PingBot(ctx context.Context) error {
	if h.bot == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		_, err := h.bot.API().GetMe()
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("Bot API не ответил вовремя")
	}
}

func (h *health) SchemaVersion() int64 {
	v, err := migrate.Version(h.db)
	if err != nil {
		return 0
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
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
