package storage

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Postgres хранит фото товаров в БД. Нужен там, где нет постоянного диска
// (бесплатные тарифы PaaS): при рестарте контейнера файлы на диске пропадают,
// а строки в базе — нет. Фото немного и они небольшие, так что bytea уместен.
type Postgres struct {
	DB      *gorm.DB
	BaseURL string // префикс URL, например /uploads
}

// NewPostgres — таблицу uploads создаёт миграция 00001_baseline, здесь
// схему не трогаем: единственный источник правды о ней — /migrations.
func NewPostgres(db *gorm.DB, baseURL string) *Postgres {
	return &Postgres{DB: db, BaseURL: strings.TrimSuffix(baseURL, "/")}
}

// Фото принимаются файлом в оригинале, поэтому лимит равен пределу,
// до которого Telegram вообще отдаёт файлы ботам.
const maxUploadBytes = 20 << 20

func (p *Postgres) Save(name string, r io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxUploadBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxUploadBytes {
		return "", fmt.Errorf("storage: фото больше %d МБ", maxUploadBytes>>20)
	}

	ext := filepath.Ext(name)
	// Готовим снимок к витрине: ресайз под экран + JPEG (см. image.go).
	if prepared, newExt, err := PrepareImage(bytes.NewReader(data)); err == nil {
		data = prepared
		if newExt != "" {
			ext = newExt
		}
	}
	if ext == "" {
		ext = ".jpg"
	}
	up := &model.Upload{
		Ext:      ext,
		MimeType: http.DetectContentType(data),
		Data:     data,
	}
	if err := p.DB.Create(up).Error; err != nil {
		return "", err
	}
	return fmt.Sprintf("%s/%d%s", p.BaseURL, up.ID, ext), nil
}

// Get возвращает содержимое загруженного файла по имени вида «42.jpg».
func (p *Postgres) Get(fileName string) (*model.Upload, error) {
	idPart := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	var up model.Upload
	if err := p.DB.Where("id = ?", idPart).First(&up).Error; err != nil {
		return nil, err
	}
	return &up, nil
}
