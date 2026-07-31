package config

import (
	"testing"
	"time"
)

// setEnv расставляет переменные окружения на время теста.
func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	// Гарантируем чистый лист: остатки от предыдущего теста не должны протекать.
	for _, k := range []string{
		"DATABASE_URL", "PORT", "TELEGRAM_BOT_TOKEN", "TELEGRAM_ADMIN_IDS",
		"TELEGRAM_WEBHOOK_SECRET", "BOT_MODE", "UPLOAD_STORE", "UPLOAD_DIR",
		"WEB_DIST", "PUBLIC_URL", "RAILWAY_PUBLIC_DOMAIN", "APP_TIMEZONE",
		"SHOP_OPEN_HOUR", "SHOP_CLOSE_HOUR", "BACKUP_ENABLED", "BACKUP_HOUR",
	} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func minimal() map[string]string {
	return map[string]string{"DATABASE_URL": "postgres://localhost/flowix"}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	setEnv(t, map[string]string{})
	if _, err := Load(); err == nil {
		t.Fatal("без DATABASE_URL конфигурация не должна загружаться")
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, minimal())
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Port != "8080" {
		t.Errorf("Port = %q", c.Port)
	}
	if c.UploadStore != "db" {
		t.Errorf("UploadStore = %q — по умолчанию фото должны храниться в БД", c.UploadStore)
	}
	if c.Location.String() != "Europe/Moscow" {
		t.Errorf("Location = %q", c.Location.String())
	}
	if c.ShopOpenHour != 9 || c.ShopCloseHour != 21 {
		t.Errorf("часы работы = %d–%d", c.ShopOpenHour, c.ShopCloseHour)
	}
	// Без PUBLIC_URL webhook зарегистрировать невозможно — честно откатываемся.
	if c.BotMode != "polling" {
		t.Errorf("BotMode = %q, без PUBLIC_URL ожидали polling", c.BotMode)
	}
	// Бэкапы без бота и админов отправлять некуда.
	if c.BackupEnabled {
		t.Error("бэкапы не должны включаться без бота и администраторов")
	}
	if c.BotEnabled() {
		t.Error("без токена бот не должен считаться включённым")
	}
}

func TestLoadPublicURLNormalization(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"домен без схемы":    {"flowix.up.railway.app", "https://flowix.up.railway.app"},
		"полный адрес":       {"https://shop.example.com", "https://shop.example.com"},
		"со слэшем на конце": {"https://shop.example.com/", "https://shop.example.com"},
		"с пробелами":        {"  flowix.up.railway.app  ", "https://flowix.up.railway.app"},
	}
	for name, c := range cases {
		env := minimal()
		env["PUBLIC_URL"] = c.in
		setEnv(t, env)
		cfg, err := Load()
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if cfg.PublicURL != c.want {
			t.Errorf("%s: PublicURL = %q, want %q", name, cfg.PublicURL, c.want)
		}
	}
}

// Railway подставляет домен сам — руками вписывать PUBLIC_URL не нужно.
func TestLoadFallsBackToRailwayDomain(t *testing.T) {
	env := minimal()
	env["RAILWAY_PUBLIC_DOMAIN"] = "flowix-production.up.railway.app"
	setEnv(t, env)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.PublicURL != "https://flowix-production.up.railway.app" {
		t.Errorf("PublicURL = %q", c.PublicURL)
	}
	// Явный PUBLIC_URL приоритетнее.
	env["PUBLIC_URL"] = "https://shop.example.com"
	setEnv(t, env)
	c, _ = Load()
	if c.PublicURL != "https://shop.example.com" {
		t.Errorf("явный PUBLIC_URL проигнорирован: %q", c.PublicURL)
	}
}

func TestLoadAdminIDs(t *testing.T) {
	env := minimal()
	env["TELEGRAM_ADMIN_IDS"] = " 111 , 222,111 ,, 333 "
	setEnv(t, env)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []int64{111, 222, 333} // дубли схлопнуты, порядок сохранён
	if len(c.AdminIDs) != len(want) {
		t.Fatalf("AdminIDs = %v, want %v", c.AdminIDs, want)
	}
	for i := range want {
		if c.AdminIDs[i] != want[i] {
			t.Fatalf("AdminIDs = %v, want %v", c.AdminIDs, want)
		}
	}
}

// Мусор в списке админов должен ронять старт, а не молча оставлять магазин
// без получателя заказов.
func TestLoadRejectsBadAdminIDs(t *testing.T) {
	for _, bad := range []string{"abc", "111,abc", "0", "111,0"} {
		env := minimal()
		env["TELEGRAM_ADMIN_IDS"] = bad
		setEnv(t, env)
		if _, err := Load(); err == nil {
			t.Errorf("TELEGRAM_ADMIN_IDS=%q должен отклоняться", bad)
		}
	}
}

func TestLoadRejectsBadValues(t *testing.T) {
	cases := map[string]map[string]string{
		"неизвестный режим бота":  {"BOT_MODE": "carrier-pigeon", "PUBLIC_URL": "https://x.dev"},
		"неизвестное хранилище":   {"UPLOAD_STORE": "s3"},
		"несуществующий пояс":     {"APP_TIMEZONE": "Mars/Olympus"},
		"открытие позже закрытия": {"SHOP_OPEN_HOUR": "22", "SHOP_CLOSE_HOUR": "9"},
		"час не число":            {"SHOP_OPEN_HOUR": "утро"},
		"час бэкапа вне суток":    {"BACKUP_HOUR": "99"},
	}
	for name, extra := range cases {
		env := minimal()
		for k, v := range extra {
			env[k] = v
		}
		setEnv(t, env)
		if _, err := Load(); err == nil {
			t.Errorf("%s: ожидали ошибку конфигурации", name)
		}
	}
}

func TestBackupEnabledRequiresBotAndAdmins(t *testing.T) {
	env := minimal()
	env["TELEGRAM_BOT_TOKEN"] = "123:TEST"
	env["TELEGRAM_ADMIN_IDS"] = "111"
	setEnv(t, env)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.BackupEnabled {
		t.Error("с ботом и админами бэкапы должны быть включены по умолчанию")
	}

	env["BACKUP_ENABLED"] = "false"
	setEnv(t, env)
	c, _ = Load()
	if c.BackupEnabled {
		t.Error("BACKUP_ENABLED=false должен выключать бэкапы")
	}
}

// Часы магазина — в его часовом поясе, а не в UTC контейнера.
func TestNowUsesShopTimezone(t *testing.T) {
	env := minimal()
	env["APP_TIMEZONE"] = "Europe/Moscow"
	setEnv(t, env)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	now := c.Now()
	if now.Location().String() != "Europe/Moscow" {
		t.Errorf("Now() вернул время в поясе %q", now.Location())
	}
	if got, want := c.Today(), now.Format("2006-01-02"); got != want {
		t.Errorf("Today() = %q, want %q", got, want)
	}
	// Разница с UTC должна быть ненулевой — иначе пояс не применился.
	_, offset := now.Zone()
	if offset != int((3 * time.Hour).Seconds()) {
		t.Errorf("смещение = %d секунд, ожидали +3 часа", offset)
	}
}
