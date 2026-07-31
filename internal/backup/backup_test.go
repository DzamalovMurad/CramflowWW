package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/dzamalovmurad/cramflowww/internal/migrate"
)

// TestIntegrationDump проверяет реальный pg_dump: без него бэкапы — фикция.
// Запуск: TEST_DATABASE_URL=postgres://... go test ./internal/backup/
func TestIntegrationDump(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	// Дамп пустой базы прошёл бы проверку «файл создался», ничего не доказав,
	// поэтому сначала раскатываем схему.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("подключение к БД: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	if err := migrate.Up(sqlDB); err != nil {
		t.Fatalf("миграции: %v", err)
	}

	dir := t.TempDir()
	s := &Service{DatabaseURL: dsn, Dir: dir}

	path := filepath.Join(dir, "test.sql.gz")
	if err := s.dump(context.Background(), path); err != nil {
		t.Fatalf("dump: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("открыть дамп: %v", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("дамп не является gzip: %v", err)
	}
	body, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("распаковка: %v", err)
	}
	// Дамп обязан содержать схему витрины, иначе восстанавливать будет нечего.
	for _, want := range []string{"CREATE TABLE public.products", "CREATE TABLE public.orders"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("в дампе нет %q", want)
		}
	}
}

func TestCleanupKeepsLastN(t *testing.T) {
	dir := t.TempDir()
	// 20 дампов: имена сортируются по дате, старые должны уйти.
	for i := 1; i <= 20; i++ {
		name := fmt.Sprintf("cramflow-2026-01-%02d-0300.sql.gz", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Посторонний файл не должен попасть под уборку.
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	(&Service{Dir: dir}).cleanup()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var dumps []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "cramflow-") {
			dumps = append(dumps, e.Name())
		}
	}
	if len(dumps) != keepLocal {
		t.Fatalf("осталось %d дампов, ожидали %d", len(dumps), keepLocal)
	}
	// Остаться должны самые свежие: с 07 по 20.
	for _, name := range dumps {
		if name < "cramflow-2026-01-07" {
			t.Errorf("старый дамп %s не удалён", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "readme.txt")); err != nil {
		t.Error("уборка задела посторонний файл")
	}
}
