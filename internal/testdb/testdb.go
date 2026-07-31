// Package testdb — общая обвязка интеграционных тестов: подключение к реальному
// Postgres, применение миграций и очистка между тестами.
//
// Без TEST_DATABASE_URL интеграционные тесты пропускаются, поэтому обычный
// `go test ./...` остаётся быстрым и не требует базы.
//
// Каждый тестовый пакет — отдельный процесс, и каждому выдаётся собственная
// схема Postgres. Иначе `go test ./...` запускает пакеты параллельно, и они
// вычищают данные друг у друга посреди теста.
package testdb

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"

	"github.com/dzamalovmurad/cramflowww/internal/migrate"
	"github.com/dzamalovmurad/cramflowww/migrations"
)

var (
	once      sync.Once
	setupErr  error
	schemaDSN string
	schema    string
)

// prepare один раз на процесс создаёт чистую схему и возвращает DSN,
// у которого search_path указывает на неё.
func prepare(dsn string) (string, error) {
	schema = fmt.Sprintf("test_%d", os.Getpid())

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		return "", fmt.Errorf("подключение к тестовой БД: %w", err)
	}
	defer admin.Close()

	if _, err := admin.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`); err != nil {
		return "", fmt.Errorf("очистка схемы: %w", err)
	}
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		return "", fmt.Errorf("создание схемы: %w", err)
	}
	return withSearchPath(dsn, schema), nil
}

// withSearchPath прописывает схему через параметр options.
//
// Именно options, а не search_path: search_path понимает pgx, но не libpq,
// а на этой же строке подключения работает pg_dump в тестах бэкапа.
func withSearchPath(dsn, schema string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	// QueryEscape кодирует пробел как «+», а libpq ждёт «%20».
	opts := strings.ReplaceAll(url.QueryEscape("-c search_path="+schema), "+", "%20")
	return dsn + sep + "options=" + opts
}

func dsnOrSkip(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	once.Do(func() { schemaDSN, setupErr = prepare(dsn) })
	if setupErr != nil {
		t.Fatalf("подготовка тестовой схемы: %v", setupErr)
	}
	return schemaDSN
}

// DSN — строка подключения к изолированной схеме этого процесса.
func DSN(t *testing.T) string { return dsnOrSkip(t) }

// NewDatabase создаёт отдельную пустую базу и возвращает строку подключения
// к ней. Нужна там, где схемы недостаточно: pg_dump/pg_restore работают
// с базой целиком, и проверять восстановление надо ровно так же, как в бою.
// База удаляется по завершении теста.
func NewDatabase(t *testing.T, suffix string) string {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	name := fmt.Sprintf("flowix_%s_%d", suffix, os.Getpid())

	// Своё подключение на каждую операцию: t.Cleanup сработает уже после
	// того, как основное соединение теста будет закрыто.
	exec := func(query string) error {
		admin, err := sql.Open("pgx", base)
		if err != nil {
			return err
		}
		defer admin.Close()
		_, err = admin.Exec(query)
		return err
	}

	drop := func() {
		if err := exec(`DROP DATABASE IF EXISTS ` + name + ` WITH (FORCE)`); err != nil {
			t.Logf("не удалось удалить тестовую базу %s: %v", name, err)
		}
	}
	drop()
	if err := exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatalf("создание тестовой базы: %v", err)
	}
	t.Cleanup(drop)

	return replaceDatabase(base, name)
}

// replaceDatabase подменяет имя базы в строке подключения.
func replaceDatabase(dsn, name string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	u.Path = "/" + name
	return u.String()
}

// OpenSQL отдаёт database/sql-подключение к изолированной схеме.
// Миграции не применяются — это для тестов самого механизма миграций.
func OpenSQL(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", dsnOrSkip(t))
	if err != nil {
		t.Fatalf("подключение к тестовой БД: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// Open подключается к тестовой схеме, применяет миграции и чистит данные.
func Open(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(dsnOrSkip(t)), &gorm.Config{
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	if err != nil {
		t.Fatalf("подключение к тестовой БД: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	if err := migrate.Run(context.Background(), sqlDB, migrations.FS, log); err != nil {
		t.Fatalf("миграции: %v", err)
	}
	Truncate(t, db)
	return db
}

// Truncate возвращает базу в чистое состояние. Порядок не важен: CASCADE
// разбирается со связями сам, RESTART IDENTITY обнуляет счётчики id.
func Truncate(t *testing.T, db *gorm.DB) {
	t.Helper()
	err := db.Exec(`TRUNCATE promo_redemptions, order_status_logs, order_items, orders,
		product_images, product_variants, products, promo_codes, users,
		fresh_todays, uploads RESTART IDENTITY CASCADE`).Error
	if err != nil {
		t.Fatalf("очистка тестовой БД: %v", err)
	}
}
