package migrate_test

import (
	"database/sql"
	"io/fs"
	"log/slog"
	"testing"
	"testing/fstest"

	"github.com/dzamalovmurad/cramflowww/internal/migrate"
	"github.com/dzamalovmurad/cramflowww/internal/testdb"
	"github.com/dzamalovmurad/cramflowww/migrations"
)

// Миграции обязаны применяться к пустой базе ровно один раз и быть
// безопасными при повторном запуске (каждый рестарт сервиса их вызывает).
func TestIntegrationMigrationsApplyOnceToEmptyDatabase(t *testing.T) {
	// Схема этого процесса создаётся пустой — ровно как при первом деплое.
	db := testdb.OpenSQL(t)
	log := slog.New(slog.DiscardHandler)
	ctx := t.Context()

	if err := migrate.Run(ctx, db, migrations.FS, log); err != nil {
		t.Fatalf("первый прогон миграций: %v", err)
	}

	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("чтение schema_migrations: %v", err)
	}
	if applied == 0 {
		t.Fatal("не применено ни одной миграции")
	}

	// Повторный прогон ничего не делает и не падает.
	for i := 0; i < 3; i++ {
		if err := migrate.Run(ctx, db, migrations.FS, log); err != nil {
			t.Fatalf("повторный прогон %d: %v", i+1, err)
		}
	}
	var appliedAgain int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&appliedAgain); err != nil {
		t.Fatalf("чтение schema_migrations: %v", err)
	}
	if appliedAgain != applied {
		t.Fatalf("повторный прогон применил миграции ещё раз: было %d, стало %d", applied, appliedAgain)
	}

	// Все таблицы, которыми пользуется приложение, должны существовать.
	want := []string{
		"products", "product_variants", "product_images", "promo_codes",
		"promo_redemptions", "users", "orders", "order_items",
		"order_status_logs", "fresh_todays", "uploads", "schema_migrations",
	}
	for _, table := range want {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = current_schema() AND table_name = $1)`, table).Scan(&exists)
		if err != nil {
			t.Fatalf("проверка таблицы %s: %v", table, err)
		}
		if !exists {
			t.Errorf("таблица %s не создана", table)
		}
	}

	// Индексы, от которых зависит производительность и корректность.
	wantIndexes := []string{
		"idx_orders_idempotency", // без него двойной тап создаёт два заказа
		"idx_order_items_variant_id",
		"idx_products_visible",
		"idx_promo_codes_upper",
		"idx_promo_redemptions_user",
	}
	for _, idx := range wantIndexes {
		var exists bool
		err := db.QueryRow(`SELECT EXISTS (
			SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1)`, idx).Scan(&exists)
		if err != nil {
			t.Fatalf("проверка индекса %s: %v", idx, err)
		}
		if !exists {
			t.Errorf("индекс %s не создан", idx)
		}
	}
}

// Перевод промокодов на типы скидок обязан сохранить уже выданные акции:
// процент из старой колонки становится значением скидки, а не обнуляется.
func TestIntegrationPromoDiscountBackfill(t *testing.T) {
	// Отдельная пустая база: этот тест проходит версии по порядку, а общая
	// схема процесса к этому моменту уже мигрирована до конца.
	db, err := sql.Open("pgx", testdb.NewDatabase(t, "migbackfill"))
	if err != nil {
		t.Fatalf("подключение: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	log := slog.New(slog.DiscardHandler)
	ctx := t.Context()

	// Состояние базы до этой миграции: применена только первая.
	if err := migrate.Run(ctx, db, onlyFiles(t, "0001_init.sql"), log); err != nil {
		t.Fatalf("первая миграция: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO promo_codes (code, discount_percent, per_user_limit, is_active)
		VALUES ('LEGACY15', 15, 1, TRUE)`); err != nil {
		t.Fatalf("промокод старого формата: %v", err)
	}

	if err := migrate.Run(ctx, db, migrations.FS, log); err != nil {
		t.Fatalf("остальные миграции: %v", err)
	}

	var typ string
	var value, minOrder int
	if err := db.QueryRowContext(ctx, `
		SELECT discount_type, discount_value, min_order_amount
		  FROM promo_codes WHERE code = 'LEGACY15'`).Scan(&typ, &value, &minOrder); err != nil {
		t.Fatalf("чтение промокода: %v", err)
	}
	if typ != "percent" || value != 15 || minOrder != 0 {
		t.Fatalf("после миграции: тип %q, значение %d, порог %d — ожидали percent/15/0", typ, value, minOrder)
	}
}

// onlyFiles — подмножество миграций, чтобы проверить переход между версиями,
// а не только чистую установку.
func onlyFiles(t *testing.T, names ...string) fs.FS {
	t.Helper()
	out := fstest.MapFS{}
	for _, name := range names {
		data, err := migrations.FS.ReadFile(name)
		if err != nil {
			t.Fatalf("чтение %s: %v", name, err)
		}
		out[name] = &fstest.MapFile{Data: data}
	}
	return out
}

// Обязательные внешние ключи: без них удаление заказа оставляет позиции-сироты.
func TestIntegrationForeignKeys(t *testing.T) {
	db := testdb.OpenSQL(t)
	if err := migrate.Run(t.Context(), db, migrations.FS, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("миграции: %v", err)
	}

	want := map[string]string{
		"order_items":       "orders",
		"product_variants":  "products",
		"product_images":    "products",
		"promo_redemptions": "orders",
		"order_status_logs": "orders",
	}
	for child, parent := range want {
		var n int
		err := db.QueryRow(`
			SELECT COUNT(*) FROM information_schema.table_constraints tc
			JOIN information_schema.constraint_column_usage ccu
			  ON tc.constraint_name = ccu.constraint_name
			WHERE tc.constraint_type = 'FOREIGN KEY'
			  AND tc.table_name = $1 AND ccu.table_name = $2`, child, parent).Scan(&n)
		if err != nil {
			t.Fatalf("проверка FK %s → %s: %v", child, parent, err)
		}
		if n == 0 {
			t.Errorf("нет внешнего ключа %s → %s", child, parent)
		}
	}
}
