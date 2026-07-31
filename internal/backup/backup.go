// Package backup — ночные дампы БД в приватный Telegram-канал.
//
// Схема простая и без внешних сервисов: pg_dump → gzip → sendDocument
// в приватный канал BACKUP_CHANNEL_ID. Телеграм хранит файлы бессрочно,
// так что канал заодно и архив; локально держим последние 14 дампов,
// чтобы восстановиться, не выходя из контейнера.
package backup

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/observability"
)

// keepLocal — сколько последних дампов держим на диске.
const keepLocal = 14

// dumpTimeout — pg_dump на этой базе укладывается в секунды; минута с запасом
// покрывает холодный старт serverless-Postgres.
const dumpTimeout = 3 * time.Minute

type Service struct {
	DatabaseURL string
	Dir         string // куда складывать дампы
	ChannelID   int64  // приватный канал-архив
	Bot         *tgbotapi.BotAPI
	// Notify — куда отчитаться о ручном запуске (0 = молча).
	AdminIDs []int64
}

// New собирает сервис бэкапов. Возвращает nil, если он не сконфигурирован:
// без канала складывать дампы некуда, а держать их только в эфемерном
// контейнере бессмысленно.
func New(databaseURL, dir string, channelID int64, bot *tgbotapi.BotAPI, adminIDs []int64) *Service {
	if databaseURL == "" || channelID == 0 || bot == nil {
		return nil
	}
	if dir == "" {
		dir = "./backups"
	}
	return &Service{
		DatabaseURL: databaseURL,
		Dir:         dir,
		ChannelID:   channelID,
		Bot:         bot,
		AdminIDs:    adminIDs,
	}
}

// Run делает дамп, сжимает, отправляет в канал и подчищает старые файлы.
// Возвращает путь к локальному файлу.
func (s *Service) Run(ctx context.Context) (string, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", fmt.Errorf("каталог бэкапов: %w", err)
	}

	name := fmt.Sprintf("cramflow-%s.sql.gz", time.Now().UTC().Format("2006-01-02-1504"))
	path := filepath.Join(s.Dir, name)

	if err := s.dump(ctx, path); err != nil {
		return "", err
	}

	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	log.Printf("бэкап: %s (%.1f МБ)", name, float64(info.Size())/(1<<20))

	if err := s.upload(path, info.Size()); err != nil {
		// Файл на диске уже есть — сообщаем об ошибке, но не считаем дамп потерянным.
		return path, fmt.Errorf("отправка в Telegram: %w", err)
	}
	s.cleanup()
	return path, nil
}

// dump запускает pg_dump и пишет сжатый вывод в файл.
// Пишем через gzip.Writer, а не через `pg_dump | gzip`: не нужен шелл,
// а ошибка pg_dump не теряется в конвейере.
func (s *Service) dump(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, dumpTimeout)
	defer cancel()

	// --no-owner/--no-privileges: восстанавливаем в базу другого хостинга,
	// где ролей исходного сервера нет.
	cmd := exec.CommandContext(ctx, "pg_dump",
		"--no-owner", "--no-privileges", "--clean", "--if-exists",
		s.DatabaseURL)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	if err := cmd.Start(); err != nil {
		if strings.Contains(err.Error(), "executable file not found") {
			return fmt.Errorf("pg_dump не установлен в образе (см. Dockerfile): %w", err)
		}
		return err
	}
	if _, err := io.Copy(gz, stdout); err != nil {
		_ = cmd.Wait()
		return err
	}
	if err := gz.Close(); err != nil {
		_ = cmd.Wait()
		return err
	}
	if err := cmd.Wait(); err != nil {
		os.Remove(path)
		return fmt.Errorf("pg_dump: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// telegramFileLimit — предел sendDocument для ботов.
const telegramFileLimit = 50 << 20

// upload отправляет дамп документом в приватный канал.
func (s *Service) upload(path string, size int64) error {
	if size > telegramFileLimit {
		return fmt.Errorf("дамп %.1f МБ больше лимита Telegram в 50 МБ — заберите файл с диска (%s)",
			float64(size)/(1<<20), path)
	}
	doc := tgbotapi.NewDocument(s.ChannelID, tgbotapi.FilePath(path))
	doc.Caption = fmt.Sprintf("💾 Бэкап CramFlow\n%s · %.1f МБ\nВосстановление — см. README.",
		time.Now().UTC().Format("2006-01-02 15:04 UTC"), float64(size)/(1<<20))
	_, err := s.Bot.Send(doc)
	return err
}

// cleanup оставляет на диске только keepLocal последних дампов.
func (s *Service) cleanup() {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return
	}
	var dumps []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "cramflow-") && strings.HasSuffix(e.Name(), ".sql.gz") {
			dumps = append(dumps, e.Name())
		}
	}
	if len(dumps) <= keepLocal {
		return
	}
	// Имена содержат дату в сортируемом виде — обычной сортировки достаточно.
	sort.Strings(dumps)
	for _, name := range dumps[:len(dumps)-keepLocal] {
		if err := os.Remove(filepath.Join(s.Dir, name)); err != nil {
			log.Printf("бэкап: не удалось удалить %s: %v", name, err)
			continue
		}
		log.Printf("бэкап: удалён старый дамп %s", name)
	}
}

// RunNightly — обёртка для крона: логирует результат и уводит ошибку в Sentry.
func (s *Service) RunNightly(ctx context.Context) {
	if _, err := s.Run(ctx); err != nil {
		observability.CaptureError(err, map[string]string{"component": "backup", "kind": "nightly"})
		// Тишина в канале — плохой сигнал: пишем админам, что архива за ночь нет.
		for _, id := range s.AdminIDs {
			msg := tgbotapi.NewMessage(id, "⚠️ Ночной бэкап не удался:\n"+err.Error())
			if _, sendErr := s.Bot.Send(msg); sendErr != nil {
				log.Printf("бэкап: не удалось предупредить админа: %v", sendErr)
			}
		}
		return
	}
	log.Println("бэкап: ночная выгрузка отправлена в канал")
}

// RunManual — /backup: делает дамп и отчитывается в чат админа.
func (s *Service) RunManual(ctx context.Context, chatID int64) {
	path, err := s.Run(ctx)
	if err != nil {
		observability.CaptureError(err, map[string]string{"component": "backup", "kind": "manual"})
		if _, sendErr := s.Bot.Send(tgbotapi.NewMessage(chatID, "⚠️ Бэкап не удался:\n"+err.Error())); sendErr != nil {
			log.Printf("бэкап: %v", sendErr)
		}
		return
	}
	text := fmt.Sprintf("✅ Бэкап готов и отправлен в архивный канал.\nФайл: %s", filepath.Base(path))
	if _, err := s.Bot.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		log.Printf("бэкап: %v", err)
	}
}
