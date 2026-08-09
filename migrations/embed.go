// Package migrations содержит SQL-миграции, вшитые в бинарник.
// Это единственный механизм изменения схемы БД в проекте.
package migrations

import "embed"

// FS — все файлы миграций. Имя файла = версия, применяются по возрастанию.
//
//go:embed *.sql
var FS embed.FS
