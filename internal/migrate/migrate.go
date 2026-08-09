// Package migrate — применение SQL-миграций из встроенной файловой системы.
//
// Правила:
//   - имя файла вида NNNN_описание.sql, порядок применения — лексикографический;
//   - каждая миграция выполняется в собственной транзакции: либо целиком, либо никак;
//   - применённые версии записываются в schema_migrations и повторно не выполняются;
//   - на время работы берётся advisory lock, поэтому одновременный старт
//     нескольких экземпляров не приводит к гонке.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
)

// advisoryLockID — произвольный, но постоянный ключ блокировки миграций.
const advisoryLockID int64 = 776_150_231

// Run применяет все ещё не применённые миграции.
func Run(ctx context.Context, db *sql.DB, files fs.FS, log *slog.Logger) error {
	names, err := sqlFiles(files)
	if err != nil {
		return err
	}

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("миграции: соединение с БД: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, advisoryLockID); err != nil {
		return fmt.Errorf("миграции: блокировка: %w", err)
	}
	defer func() {
		if _, err := conn.ExecContext(context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock($1)`, advisoryLockID); err != nil {
			log.Error("не удалось снять блокировку миграций", "err", err)
		}
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return fmt.Errorf("миграции: создание schema_migrations: %w", err)
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	for _, name := range names {
		version := strings.TrimSuffix(name, ".sql")
		if applied[version] {
			continue
		}
		body, err := fs.ReadFile(files, name)
		if err != nil {
			return fmt.Errorf("миграции: чтение %s: %w", name, err)
		}
		if err := applyOne(ctx, conn, version, string(body)); err != nil {
			return err
		}
		log.Info("миграция применена", "version", version)
	}
	return nil
}

func applyOne(ctx context.Context, conn *sql.Conn, version, body string) error {
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("миграции: %s: начало транзакции: %w", version, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("миграции: %s: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return fmt.Errorf("миграции: %s: отметка версии: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("миграции: %s: commit: %w", version, err)
	}
	return nil
}

func appliedVersions(ctx context.Context, conn *sql.Conn) (map[string]bool, error) {
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations`)
	if err != nil {
		return nil, fmt.Errorf("миграции: чтение применённых версий: %w", err)
	}
	defer rows.Close()

	applied := map[string]bool{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("миграции: разбор версии: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("миграции: чтение применённых версий: %w", err)
	}
	return applied, nil
}

func sqlFiles(files fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("миграции: чтение каталога: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return nil, errors.New("миграции: не найдено ни одного .sql-файла")
	}
	sort.Strings(names)
	return names, nil
}
