// Команда flowix — единый бинарник магазина: HTTP API, статика Mini App,
// Telegram-бот и фоновые задания.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/dzamalovmurad/cramflowww/internal/backup"
	"github.com/dzamalovmurad/cramflowww/internal/config"
	"github.com/dzamalovmurad/cramflowww/internal/handler"
	"github.com/dzamalovmurad/cramflowww/internal/migrate"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
	"github.com/dzamalovmurad/cramflowww/migrations"
)

// shutdownGrace — сколько даём на завершение начатых запросов и отправок.
const shutdownGrace = 20 * time.Second

func main() {
	seed := flag.Bool("seed", false, "заполнить пустую БД демо-товарами и выйти")
	backupNow := flag.Bool("backup", false, "снять резервную копию, отправить админам и выйти")
	flag.Parse()

	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(log)

	if err := run(log, *seed, *backupNow); err != nil {
		log.Error("сервис остановлен с ошибкой", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, seed, backupNow bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, err := openDB(cfg, log)
	if err != nil {
		return err
	}
	defer closeDB(db, log)

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	// Единственный механизм миграций: SQL-файлы из migrations/, вшитые в бинарник.
	migCtx, cancelMig := context.WithTimeout(context.Background(), 2*time.Minute)
	err = migrate.Run(migCtx, sqlDB, migrations.FS, log)
	cancelMig()
	if err != nil {
		return err
	}

	repo := repository.New(db, cfg.Now)
	svc := service.New(repo, cfg, log)

	if seed {
		return runSeed(context.Background(), repo, log)
	}

	store, uploads, err := newStorage(cfg, db, log)
	if err != nil {
		return err
	}

	var bot *handler.Bot
	if cfg.BotEnabled() {
		if bot, err = handler.NewBot(cfg, log, repo, svc, store); err != nil {
			return err
		}
		if len(cfg.AdminIDs) == 0 {
			log.Warn("TELEGRAM_ADMIN_IDS не заданы — админ-команды и уведомления о заказах недоступны")
		}
	} else {
		log.Warn("TELEGRAM_BOT_TOKEN не задан — запуск без бота, оформление заказов работать не будет")
	}

	if backupNow {
		if bot == nil {
			return errors.New("резервная копия отправляется в Telegram: нужны TELEGRAM_BOT_TOKEN и TELEGRAM_ADMIN_IDS")
		}
		runner := &backup.Runner{
			DatabaseURL: cfg.DatabaseURL, Location: cfg.Location,
			Hour: cfg.BackupHour, Sender: bot, Log: log,
		}
		return runner.Once(context.Background())
	}

	api := &handler.API{Repo: repo, Service: svc, Cfg: cfg, Log: log, Uploads: uploads}
	defer api.Close()

	// Контекст жизни процесса: отменяется по SIGINT/SIGTERM (Railway шлёт SIGTERM).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	if bot != nil {
		if err := startBot(&wg, cfg, api, bot, log); err != nil {
			return err
		}
		if cfg.BackupEnabled {
			runner := &backup.Runner{
				DatabaseURL: cfg.DatabaseURL, Location: cfg.Location,
				Hour: cfg.BackupHour, Sender: bot, Log: log,
			}
			wg.Add(1)
			go func() { defer wg.Done(); runner.Run(ctx) }()
		}
	}

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: api.Routes(),
		// Без этих таймаутов достаточно нескольких «медленных» соединений,
		// чтобы занять все горутины сервера.
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("HTTP-сервер запущен",
			"port", cfg.Port, "public_url", cfg.PublicURL,
			"bot_mode", cfg.BotMode, "upload_store", cfg.UploadStore, "tz", cfg.Location.String())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Info("получен сигнал остановки, завершаем начатое")
	}

	// Сначала перестаём принимать новое и даём доиграть текущим запросам,
	// потом останавливаем бота и фоновые задания.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("HTTP-сервер не завершился штатно", "err", err)
	}
	// Порядок важен: сначала перестаём принимать заказы, потом даём
	// разойтись уведомлениям о уже принятых, и только затем гасим бота.
	svc.DrainNotifications(shutdownCtx)
	if bot != nil {
		bot.Stop(shutdownCtx)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-shutdownCtx.Done():
		log.Warn("фоновые задания не успели завершиться")
	}
	log.Info("сервис остановлен")
	return nil
}

// openDB подключается к Postgres и настраивает пул.
func openDB(cfg *config.Config, log *slog.Logger) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{
		Logger: glogger.Default.LogMode(glogger.Warn),
		// FK и индексы создают миграции; GORM не должен трогать схему.
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// Небольшой пул: нагрузка магазина измеряется десятками запросов в минуту,
	// а лишние соединения только жгут лимиты управляемого Postgres.
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(4)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, err
	}
	log.Info("подключение к БД установлено")
	return db, nil
}

func closeDB(db *gorm.DB, log *slog.Logger) {
	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			log.Error("не удалось закрыть соединение с БД", "err", err)
		}
	}
}

// newStorage выбирает хранилище фото товаров.
func newStorage(cfg *config.Config, db *gorm.DB, log *slog.Logger) (storage.Storage, *storage.Postgres, error) {
	if cfg.UploadStore == "db" {
		pg := storage.NewPostgres(db, "/uploads")
		log.Info("фото товаров хранятся в БД")
		return pg, pg, nil
	}
	local, err := storage.NewLocal(cfg.UploadDir, "/uploads")
	if err != nil {
		return nil, nil, err
	}
	log.Warn("фото товаров хранятся на диске — без подключённого Volume они пропадут при редеплое",
		"dir", cfg.UploadDir)
	return local, nil, nil
}

// startBot включает выбранный режим приёма апдейтов.
func startBot(wg *sync.WaitGroup, cfg *config.Config, api *handler.API, bot *handler.Bot, log *slog.Logger) error {
	log = log.With("bot_username", bot.Username())

	if cfg.BotMode == "webhook" {
		secret := cfg.WebhookSecret
		if secret == "" {
			var err error
			if secret, err = handler.NewWebhookSecret(); err != nil {
				return err
			}
			log.Info("TELEGRAM_WEBHOOK_SECRET не задан — сгенерирован временный на время работы процесса")
		}
		api.WebhookPath = bot.WebhookPath(secret)
		api.WebhookHandler = bot.WebhookHandler(secret)
		if err := bot.SetupWebhook(cfg.PublicURL, secret); err != nil {
			return err
		}
		return nil
	}

	if err := bot.RemoveWebhook(); err != nil {
		log.Warn("не удалось снять webhook перед long polling", "err", err)
	}
	wg.Add(1)
	go func() { defer wg.Done(); bot.Run() }()
	return nil
}
