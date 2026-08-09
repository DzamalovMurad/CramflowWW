package backup_test

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/dzamalovmurad/cramflowww/internal/backup"
	"github.com/dzamalovmurad/cramflowww/internal/migrate"
	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/testdb"
	"github.com/dzamalovmurad/cramflowww/migrations"
)

// fakeSender ловит дамп вместо отправки в Telegram.
type fakeSender struct {
	name    string
	data    []byte
	caption string
	err     error
	calls   int
}

func (s *fakeSender) SendBackup(name string, data []byte, caption string) error {
	s.calls++
	s.name, s.data, s.caption = name, data, caption
	return s.err
}

// Полный цикл: заполняем базу → снимаем дамп → сносим данные →
// восстанавливаем pg_restore → проверяем, что заказ на месте.
//
// Это ровно та процедура, что описана в README. Без такой проверки бэкап
// остаётся обещанием, а не гарантией.
func TestIntegrationBackupAndRestore(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump не установлен — тест бэкапа пропущен")
	}
	// Отдельная база: pg_dump/pg_restore работают с базой целиком, и проверять
	// восстановление надо ровно так же, как это будет в бою.
	dsn := testdb.NewDatabase(t, "backup")
	db := openMigrated(t, dsn)

	// 1. Данные, которые должны пережить катастрофу.
	product := &model.Product{
		Name: "Букет для бэкапа", Category: model.CategoryPremium,
		Variants: []model.ProductVariant{{Quantity: 9, Price: 4242}},
	}
	if err := db.Create(product).Error; err != nil {
		t.Fatalf("товар: %v", err)
	}
	user := &model.User{TelegramID: 987654, Name: "Клиент Бэкапов", Phone: "+79001112233"}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("клиент: %v", err)
	}
	order := &model.Order{
		UserID: user.ID, SubtotalPrice: 4242, TotalPrice: 4242,
		DeliveryAddress: "Москва, ул. Восстановленная, 1",
		DeliveryDate:    "2026-08-15", DeliveryTime: "к 15:00", Status: model.StatusNew,
		Items: []model.OrderItem{{
			VariantID: product.Variants[0].ID, Quantity: 1, Price: 4242,
			ProductName: product.Name,
		}},
	}
	if err := db.Create(order).Error; err != nil {
		t.Fatalf("заказ: %v", err)
	}

	// 2. Снимаем дамп ровно так, как это делает фоновое задание.
	sender := &fakeSender{}
	runner := &backup.Runner{
		DatabaseURL: dsn, Location: time.UTC, Hour: 4,
		Sender: sender, Log: slog.New(slog.DiscardHandler),
	}
	if err := runner.Once(t.Context()); err != nil {
		t.Fatalf("снятие бэкапа: %v", err)
	}
	if sender.calls != 1 {
		t.Fatalf("бэкап отправлен %d раз", sender.calls)
	}
	if len(sender.data) == 0 {
		t.Fatal("дамп пуст")
	}
	if filepath.Ext(sender.name) != ".dump" {
		t.Errorf("имя файла = %q — не соответствует формату pg_restore", sender.name)
	}

	// 3. Катастрофа: базы больше нет. Восстанавливаемся в чистую —
	//    именно так, как описано в README.
	dumpPath := filepath.Join(t.TempDir(), sender.name)
	if err := os.WriteFile(dumpPath, sender.data, 0o600); err != nil {
		t.Fatalf("запись дампа: %v", err)
	}
	closeDB(t, db)

	freshDSN := testdb.NewDatabase(t, "restore")

	// 4. Восстановление — команда из README.
	cmd := exec.CommandContext(t.Context(), "pg_restore",
		"--dbname", freshDSN, "--no-owner", "--no-privileges", dumpPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pg_restore: %v\n%s", err, out)
	}

	// 5. Проверяем, что заказ действительно вернулся целиком.
	restoredDB := openDB(t, freshDSN)
	var restored model.Order
	err := restoredDB.Preload("User").Preload("Items").First(&restored, order.ID).Error
	if err != nil {
		t.Fatalf("заказ не восстановился: %v", err)
	}
	if restored.TotalPrice != 4242 {
		t.Errorf("сумма заказа = %d, ожидали 4242", restored.TotalPrice)
	}
	if restored.DeliveryAddress != "Москва, ул. Восстановленная, 1" {
		t.Errorf("адрес = %q", restored.DeliveryAddress)
	}
	if restored.User.Name != "Клиент Бэкапов" || restored.User.Phone != "+79001112233" {
		t.Errorf("клиент восстановился неполно: %+v", restored.User)
	}
	if len(restored.Items) != 1 || restored.Items[0].ProductName != "Букет для бэкапа" {
		t.Errorf("позиции заказа не восстановились: %+v", restored.Items)
	}

	// Схема тоже должна восстановиться: приложение обязано подняться поверх
	// восстановленной базы без ручных доделок.
	var applied int64
	if err := restoredDB.Raw(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied).Error; err != nil {
		t.Fatalf("schema_migrations не восстановилась: %v", err)
	}
	if applied == 0 {
		t.Error("в восстановленной базе нет отметок о миграциях")
	}
	// И повторный прогон миграций поверх восстановленной базы не должен ломаться.
	sqlDB, err := restoredDB.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	if err := migrate.Run(t.Context(), sqlDB, migrations.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("миграции поверх восстановленной базы: %v", err)
	}
	closeDB(t, restoredDB)
}

// openDB подключается к базе по DSN.
func openDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	if err != nil {
		t.Fatalf("подключение к %s: %v", dsn, err)
	}
	return db
}

// openMigrated подключается и применяет миграции.
func openMigrated(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db := openDB(t, dsn)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	if err := migrate.Run(t.Context(), sqlDB, migrations.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("миграции: %v", err)
	}
	return db
}

// closeDB закрывает пул, иначе DROP DATABASE упрётся в живые соединения.
func closeDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// Сбой отправки должен возвращаться наверх, а не молча теряться:
// «бэкап делается» и «бэкап доставлен» — разные вещи.
func TestIntegrationBackupReportsSendFailure(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump не установлен")
	}
	dsn := testdb.NewDatabase(t, "sendfail")
	closeDB(t, openMigrated(t, dsn))

	sendErr := errors.New("telegram недоступен")
	runner := &backup.Runner{
		DatabaseURL: dsn, Location: time.UTC, Hour: 4,
		Sender: &fakeSender{err: sendErr}, Log: slog.New(slog.DiscardHandler),
	}
	if err := runner.Once(t.Context()); !errors.Is(err, sendErr) {
		t.Fatalf("ошибка отправки должна всплывать, получили %v", err)
	}
}

// Некорректный DSN не должен приводить к «успешному» пустому бэкапу.
func TestBackupFailsOnBadDSN(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump не установлен")
	}
	if _, err := backup.Dump(t.Context(), "postgres://nobody@127.0.0.1:1/nothing"); err == nil {
		t.Fatal("ожидали ошибку при недоступной базе")
	}
	if _, err := backup.Dump(t.Context(), ""); err == nil {
		t.Fatal("ожидали ошибку при пустом DSN")
	}
}
