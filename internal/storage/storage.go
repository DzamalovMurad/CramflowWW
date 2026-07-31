// Package storage — единственная абстракция проекта: куда складывать фото товаров.
// Реализации: Postgres (по умолчанию, не требует диска) и Local (папка/Volume).
package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Storage сохраняет файл и возвращает публичный URL-путь (например /uploads/xxx.jpg).
type Storage interface {
	Save(ctx context.Context, name string, r io.Reader) (url string, err error)
}

// MaxUploadBytes — предел, до которого Telegram вообще отдаёт файлы ботам.
const MaxUploadBytes = 20 << 20

// Local хранит файлы в папке на диске; отдаются HTTP-сервером по BaseURL.
type Local struct {
	Dir     string
	BaseURL string
}

func NewLocal(dir, baseURL string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: mkdir %s: %w", dir, err)
	}
	return &Local{Dir: dir, BaseURL: strings.TrimSuffix(baseURL, "/")}, nil
}

func (l *Local) Save(_ context.Context, name string, r io.Reader) (string, error) {
	raw, err := readLimited(r)
	if err != nil {
		return "", err
	}
	// Готовим снимок к витрине так же, как в БД-хранилище (см. image.go).
	data, ext, err := PrepareImage(raw, filepath.Ext(name))
	if err != nil {
		return "", err
	}

	fname := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	// Пишем во временный файл и переименовываем: оборванная загрузка
	// не оставит на диске битую картинку под рабочим именем.
	tmp := filepath.Join(l.Dir, "."+fname+".part")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", fmt.Errorf("storage: запись файла: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(l.Dir, fname)); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("storage: сохранение файла: %w", err)
	}
	return l.BaseURL + "/" + fname, nil
}

// readLimited читает не больше MaxUploadBytes и честно сообщает о превышении.
func readLimited(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxUploadBytes+1))
	if err != nil {
		return nil, fmt.Errorf("storage: чтение файла: %w", err)
	}
	if len(data) > MaxUploadBytes {
		return nil, fmt.Errorf("storage: фото больше %d МБ", MaxUploadBytes>>20)
	}
	return data, nil
}
