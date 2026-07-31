package migrate_test

import (
	"log/slog"
	"testing"

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
