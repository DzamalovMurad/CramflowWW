// Package migrations хранит SQL-миграции схемы и отдаёт их как embed.FS.
//
// Файлы лежат здесь, а не внутри internal/, чтобы каталог оставался привычным
// местом для правок и работал с goose CLI напрямую. Бинарник несёт их в себе —
// рантайм-образу не нужен ни каталог с миграциями, ни установленный goose.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
