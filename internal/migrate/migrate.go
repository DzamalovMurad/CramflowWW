// Package migrate применяет SQL-миграции через goose.
//
// Почему goose, а не golang-migrate: goose запускается в процессе на уже открытом
// *sql.DB и читает миграции из embed.FS по одному файлу на версию — это ложится
// на текущую структуру (нумерованные .sql в /migrations, один бинарник без CLI
// в рантайм-образе), тогда как golang-migrate потребовал бы пары up/down-файлов
// и отдельного драйвера.
package migrate

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	"github.com/dzamalovmurad/cramflowww/migrations"
)

// dir — корень embed.FS с миграциями (файлы лежат в /migrations).
const dir = "."

func setup() error {
	goose.SetBaseFS(migrations.FS)
	goose.SetTableName("goose_db_version")
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose: диалект: %w", err)
	}
	return nil
}

// Up применяет все неприменённые миграции. Вызывается при старте сервиса:
// схема БД всегда соответствует бинарнику, который её обслуживает.
func Up(db *sql.DB) error {
	if err := setup(); err != nil {
		return err
	}
	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("goose: миграции: %w", err)
	}
	return nil
}

// Down откатывает последнюю миграцию (ручная операция, см. README).
func Down(db *sql.DB) error {
	if err := setup(); err != nil {
		return err
	}
	return goose.Down(db, dir)
}

// Status печатает список миграций и их состояние.
func Status(db *sql.DB) error {
	if err := setup(); err != nil {
		return err
	}
	return goose.Status(db, dir)
}

// Version — текущая версия схемы (показывается в /healthz).
func Version(db *sql.DB) (int64, error) {
	if err := setup(); err != nil {
		return 0, err
	}
	return goose.GetDBVersion(db)
}
