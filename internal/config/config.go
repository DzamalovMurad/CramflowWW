// Package config — единственное место, где читаются переменные окружения.
// Никаких os.Getenv в остальном коде: так видно весь конфигурационный
// контракт сервиса и его легко описать в .env.example и README.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — полная конфигурация сервиса.
type Config struct {
	DatabaseURL string
	Port        string

	// PublicURL — публичный https-адрес сервиса. Из него собираются
	// адрес Mini App, webhook бота и deep-link-и.
	PublicURL string

	BotToken      string
	AdminIDs      []int64
	WebhookSecret string
	BotMode       string // webhook | polling

	// WelcomePhoto — снимок для первого экрана бота: file_id уже загруженного
	// в Telegram фото или прямая https-ссылка. Пусто — приветствие уходит текстом.
	WelcomePhoto string

	UploadStore string // db | local
	UploadDir   string
	WebDist     string

	Location      *time.Location
	ShopOpenHour  int
	ShopCloseHour int

	BackupEnabled bool
	BackupHour    int // час по местному времени магазина

	// InitDataTTL — максимальный возраст подписи initData Telegram.
	InitDataTTL time.Duration
}

// Load читает и валидирует окружение. Возвращает ошибку, если сервис
// заведомо не сможет работать корректно, — падать лучше на старте.
func Load() (*Config, error) {
	c := &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		Port:          envOr("PORT", "8080"),
		BotToken:      strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		WebhookSecret: strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET")),
		BotMode:       strings.ToLower(envOr("BOT_MODE", "webhook")),
		WelcomePhoto:  strings.TrimSpace(os.Getenv("WELCOME_PHOTO")),
		UploadStore:   strings.ToLower(envOr("UPLOAD_STORE", "db")),
		UploadDir:     envOr("UPLOAD_DIR", "./uploads"),
		WebDist:       envOr("WEB_DIST", "./web/dist"),
		InitDataTTL:   24 * time.Hour,
	}

	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL не задан")
	}

	c.PublicURL = normalizeURL(firstNonEmpty(
		os.Getenv("PUBLIC_URL"),
		// Railway подставляет домен сервиса сам — не заставляем вписывать руками.
		os.Getenv("RAILWAY_PUBLIC_DOMAIN"),
	))

	var err error
	if c.AdminIDs, err = parseIDs(os.Getenv("TELEGRAM_ADMIN_IDS")); err != nil {
		return nil, fmt.Errorf("TELEGRAM_ADMIN_IDS: %w", err)
	}

	tzName := envOr("APP_TIMEZONE", "Europe/Moscow")
	if c.Location, err = time.LoadLocation(tzName); err != nil {
		return nil, fmt.Errorf("APP_TIMEZONE=%q: %w", tzName, err)
	}

	if c.ShopOpenHour, err = envInt("SHOP_OPEN_HOUR", 9); err != nil {
		return nil, err
	}
	if c.ShopCloseHour, err = envInt("SHOP_CLOSE_HOUR", 21); err != nil {
		return nil, err
	}
	if c.ShopOpenHour < 0 || c.ShopOpenHour > 23 || c.ShopCloseHour < 1 || c.ShopCloseHour > 23 ||
		c.ShopOpenHour >= c.ShopCloseHour {
		return nil, fmt.Errorf("часы работы заданы неверно: %d–%d", c.ShopOpenHour, c.ShopCloseHour)
	}

	if c.BackupHour, err = envInt("BACKUP_HOUR", 4); err != nil {
		return nil, err
	}
	if c.BackupHour < 0 || c.BackupHour > 23 {
		return nil, fmt.Errorf("BACKUP_HOUR должен быть от 0 до 23, получено %d", c.BackupHour)
	}
	// Бэкапы уезжают в Telegram админам — без бота и админов их некуда слать.
	c.BackupEnabled = envBool("BACKUP_ENABLED", true) && c.BotToken != "" && len(c.AdminIDs) > 0

	if c.BotMode != "webhook" && c.BotMode != "polling" {
		return nil, fmt.Errorf("BOT_MODE должен быть webhook или polling, получено %q", c.BotMode)
	}
	if c.UploadStore != "db" && c.UploadStore != "local" {
		return nil, fmt.Errorf("UPLOAD_STORE должен быть db или local, получено %q", c.UploadStore)
	}

	// Webhook без публичного адреса зарегистрировать невозможно — честно
	// откатываемся на long polling, а не падаем в рестарт-луп.
	if c.BotMode == "webhook" && c.PublicURL == "" {
		c.BotMode = "polling"
	}

	return c, nil
}

// BotEnabled — бот настроен и может быть запущен.
func (c *Config) BotEnabled() bool { return c.BotToken != "" }

// Now — текущее время в часовом поясе магазина. Весь код обязан
// пользоваться им, а не time.Now(): контейнер живёт в UTC.
func (c *Config) Now() time.Time { return time.Now().In(c.Location) }

// Today — сегодняшняя дата магазина в формате YYYY-MM-DD.
func (c *Config) Today() string { return c.Now().Format("2006-01-02") }

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: ожидалось число, получено %q", key, raw)
	}
	return v, nil
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "":
		return def
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// normalizeURL приводит адрес к виду https://host без завершающего слэша.
// Railway отдаёт RAILWAY_PUBLIC_DOMAIN без схемы.
func normalizeURL(raw string) string {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return raw
}

func parseIDs(raw string) ([]int64, error) {
	var ids []int64
	seen := map[int64]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("некорректный telegram_id %q", part)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}
