// Package storage — единственная абстракция проекта: куда складывать фото.
// Сейчас — локальная папка (Railway Volume), потом можно добавить S3/R2.
package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Storage сохраняет файл и возвращает публичный URL-путь (например /uploads/xxx.jpg).
type Storage interface {
	Save(name string, r io.Reader) (url string, err error)
}

// Local хранит файлы в папке на диске; отдаются самим HTTP-сервером по baseURL.
type Local struct {
	Dir     string // папка на диске, например ./uploads
	BaseURL string // префикс URL, например /uploads
}

func NewLocal(dir, baseURL string) (*Local, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("storage: mkdir %s: %w", dir, err)
	}
	return &Local{Dir: dir, BaseURL: strings.TrimSuffix(baseURL, "/")}, nil
}

func (l *Local) Save(name string, r io.Reader) (string, error) {
	ext := filepath.Ext(name)
	// Готовим снимок к витрине так же, как в БД-хранилище (см. image.go).
	data, newExt, err := PrepareImage(r)
	if err != nil {
		return "", err
	}
	if newExt != "" {
		ext = newExt
	}
	if ext == "" {
		ext = ".jpg"
	}
	fname := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	dst, err := os.Create(filepath.Join(l.Dir, fname))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := dst.Write(data); err != nil {
		return "", err
	}
	return l.BaseURL + "/" + fname, nil
}
