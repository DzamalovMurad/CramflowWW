package storage

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Минимальный валидный PNG 1×1 — чтобы проверить определение MIME-типа.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
	0, 0, 0, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1, 0x0d, 0x0a, 0x2d, 0xb4,
	0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// Хранение фото в БД: Save → Get возвращает те же байты и корректный MIME.
// Запуск: TEST_DATABASE_URL=postgres://... go test ./internal/storage/ -run Integration
func TestIntegrationPostgresStorageRoundTrip(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("подключение к БД: %v", err)
	}
	st, err := NewPostgres(db, "/uploads")
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}

	url, err := st.Save("photo.png", bytes.NewReader(onePixelPNG))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasPrefix(url, "/uploads/") || !strings.HasSuffix(url, ".png") {
		t.Fatalf("неожиданный URL: %q", url)
	}

	up, err := st.Get(strings.TrimPrefix(url, "/uploads/"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(up.Data, onePixelPNG) {
		t.Error("содержимое файла не совпадает с исходным")
	}
	if up.MimeType != "image/png" {
		t.Errorf("MIME-тип = %q, ожидали image/png", up.MimeType)
	}

	// Несуществующий файл — ошибка, а не пустой результат.
	if _, err := st.Get("999999.png"); err == nil {
		t.Error("ожидали ошибку для несуществующего файла")
	}
}

// Слишком большое фото отклоняется, а не сохраняется целиком.
func TestIntegrationPostgresStorageTooLarge(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	st, err := NewPostgres(db, "/uploads")
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	if _, err := st.Save("big.jpg", bytes.NewReader(make([]byte, maxUploadBytes+10))); err == nil {
		t.Error("ожидали отказ для файла больше лимита")
	}
}
