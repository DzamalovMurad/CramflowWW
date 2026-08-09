package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Postgres хранит фото товаров в БД. Это режим по умолчанию: Railway без
// подключённого Volume теряет файлы на диске при каждом редеплое, и витрина
// молча остаётся без картинок. Строки в базе переживают деплой всегда.
type Postgres struct {
	DB      *gorm.DB
	BaseURL string
}

// NewPostgres — таблица uploads создаётся миграцией 0001, здесь только проверка.
func NewPostgres(db *gorm.DB, baseURL string) *Postgres {
	return &Postgres{DB: db, BaseURL: strings.TrimSuffix(baseURL, "/")}
}

func (p *Postgres) Save(ctx context.Context, name string, r io.Reader) (string, error) {
	raw, err := readLimited(r)
	if err != nil {
		return "", err
	}
	data, ext, err := PrepareImage(raw, filepath.Ext(name))
	if err != nil {
		return "", err
	}

	up := &model.Upload{
		Ext:      ext,
		MimeType: http.DetectContentType(data),
		Data:     data,
	}
	if err := p.DB.WithContext(ctx).Create(up).Error; err != nil {
		return "", fmt.Errorf("storage: сохранение фото в БД: %w", err)
	}
	return fmt.Sprintf("%s/%d%s", p.BaseURL, up.ID, ext), nil
}

// Get возвращает содержимое загруженного файла по имени вида «42.jpg».
func (p *Postgres) Get(ctx context.Context, fileName string) (*model.Upload, error) {
	idPart := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	// Имя приходит из URL — превращаем в число сами, а не отдаём строку в WHERE.
	id, err := strconv.ParseUint(idPart, 10, 64)
	if err != nil {
		return nil, errors.New("storage: некорректное имя файла")
	}
	var up model.Upload
	if err := p.DB.WithContext(ctx).First(&up, id).Error; err != nil {
		return nil, err
	}
	return &up, nil
}
