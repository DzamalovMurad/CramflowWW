// Package backup — ежедневный дамп БД, который уезжает админам в Telegram.
//
// Почему так, а не «настоящее» резервное копирование: у магазина нет ни
// объектного хранилища, ни человека, который следил бы за ним. Дамп в личном
// чате владельца — то, что реально доживёт до момента, когда понадобится,
// и восстанавливается одной командой psql (процедура — в README).
package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// Sender отправляет файл дампа (реализуется ботом).
type Sender interface {
	SendBackup(name string, data []byte, caption string) error
}

// Runner — фоновое задание резервного копирования.
type Runner struct {
	DatabaseURL string
	Location    *time.Location
	Hour        int
	Sender      Sender
	Log         *slog.Logger
}

// maxTelegramFile — предел на файл, который бот может отправить.
const maxTelegramFile = 45 << 20

// dumpTimeout — дамп маленькой базы занимает секунды; больше минуты
// означает, что что-то не так, и висеть незачем.
const dumpTimeout = 2 * time.Minute

// Run крутится до отмены контекста, снимая дамп раз в сутки в заданный час.
//
// Задание идемпотентно и не хранит состояния: если сервис перезапустили,
// следующий запуск просто произойдёт в ближайший подходящий час. Пропущенный
// из-за рестарта дамп не «теряется» — он не нужен, нужен свежий.
func (r *Runner) Run(ctx context.Context) {
	r.Log.Info("резервное копирование включено", "hour", r.Hour, "tz", r.Location.String())
	for {
		wait := r.untilNext(time.Now().In(r.Location))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			r.Log.Info("резервное копирование остановлено")
			return
		case <-timer.C:
		}
		if err := r.Once(ctx); err != nil {
			r.Log.Error("резервное копирование не удалось", "err", err)
		}
	}
}

// untilNext — сколько ждать до ближайшего запуска.
func (r *Runner) untilNext(now time.Time) time.Duration {
	next := time.Date(now.Year(), now.Month(), now.Day(), r.Hour, 0, 0, 0, r.Location)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}

// Once снимает дамп и отправляет его. Вынесено отдельно, чтобы бэкап можно
// было проверить вручную, не дожидаясь ночи.
func (r *Runner) Once(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, dumpTimeout)
	defer cancel()

	data, err := Dump(ctx, r.DatabaseURL)
	if err != nil {
		return err
	}
	if len(data) > maxTelegramFile {
		return fmt.Errorf("дамп %d МБ — больше лимита Telegram, нужен внешний бэкап",
			len(data)>>20)
	}

	stamp := time.Now().In(r.Location).Format("2006-01-02_15-04")
	name := fmt.Sprintf("flowix-%s.dump", stamp)
	caption := fmt.Sprintf("💾 Резервная копия базы\n%s · %.1f МБ\n\nВосстановление — раздел «Бэкап» в README.",
		stamp, float64(len(data))/(1<<20))

	if err := r.Sender.SendBackup(name, data, caption); err != nil {
		return fmt.Errorf("отправка дампа: %w", err)
	}
	r.Log.Info("резервная копия отправлена", "bytes", len(data), "name", name)
	return nil
}

// Dump снимает сжатый дамп базы через pg_dump.
// Бинарник pg_dump ставится в рантайм-образ (см. Dockerfile).
func Dump(ctx context.Context, databaseURL string) ([]byte, error) {
	if databaseURL == "" {
		return nil, errors.New("бэкап: DATABASE_URL пуст")
	}
	// Строка подключения передаётся аргументом, не через shell —
	// пароль не попадает ни в лог, ни в список процессов через sh -c.
	cmd := exec.CommandContext(ctx, "pg_dump",
		"--dbname", databaseURL,
		"--no-owner", "--no-privileges",
		"--format", "custom", // сжатый архив pg_restore, а не голый SQL
		"--compress", "6",
	)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		// В stderr pg_dump не печатает пароль, но URL мы не подставляем сами.
		return nil, fmt.Errorf("бэкап: pg_dump: %w: %s", err, msg)
	}
	if out.Len() == 0 {
		return nil, errors.New("бэкап: pg_dump вернул пустой файл")
	}
	return out.Bytes(), nil
}
