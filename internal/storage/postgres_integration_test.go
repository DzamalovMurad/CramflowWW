package storage_test

import (
	"bytes"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/dzamalovmurad/cramflowww/internal/storage"
	"github.com/dzamalovmurad/cramflowww/internal/testdb"
)

// Минимальный валидный PNG 1×1 — чтобы проверить определение MIME-типа.
var onePixelPNG = []byte{
	0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a,
	0, 0, 0, 0x0d, 'I', 'H', 'D', 'R', 0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0, 0x1f, 0x15, 0xc4, 0x89,
	0, 0, 0, 0x0a, 'I', 'D', 'A', 'T', 0x78, 0x9c, 0x63, 0, 1, 0, 0, 5, 0, 1, 0x0d, 0x0a, 0x2d, 0xb4,
	0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82,
}

// Хранение фото в БД: Save → Get возвращает корректную картинку и MIME.
func TestIntegrationPostgresStorageRoundTrip(t *testing.T) {
	db := testdb.Open(t)
	st := storage.NewPostgres(db, "/uploads")
	ctx := t.Context()

	url, err := st.Save(ctx, "photo.png", bytes.NewReader(onePixelPNG))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasPrefix(url, "/uploads/") || !strings.HasSuffix(url, ".jpg") {
		t.Fatalf("неожиданный URL: %q", url)
	}

	up, err := st.Get(ctx, strings.TrimPrefix(url, "/uploads/"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if up.MimeType != "image/jpeg" {
		t.Errorf("MIME = %q, ожидали image/jpeg", up.MimeType)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(up.Data))
	if err != nil {
		t.Fatalf("сохранённые данные не читаются как JPEG: %v", err)
	}
	if cfg.Width != 1 || cfg.Height != 1 {
		t.Errorf("размер изменился: %d×%d", cfg.Width, cfg.Height)
	}
}

func TestIntegrationPostgresStorageRejectsBadNames(t *testing.T) {
	db := testdb.Open(t)
	st := storage.NewPostgres(db, "/uploads")
	ctx := t.Context()

	// Имя файла приходит из URL — оно не должно уезжать в SQL как строка.
	for _, name := range []string{"999999.jpg", "../../etc/passwd", "1 OR 1=1", "", "abc.jpg"} {
		if _, err := st.Get(ctx, name); err == nil {
			t.Errorf("Get(%q) должен возвращать ошибку", name)
		}
	}
}

// Слишком большое фото отклоняется, а не сохраняется целиком.
func TestIntegrationPostgresStorageTooLarge(t *testing.T) {
	db := testdb.Open(t)
	st := storage.NewPostgres(db, "/uploads")

	_, err := st.Save(t.Context(), "big.jpg", bytes.NewReader(make([]byte, storage.MaxUploadBytes+10)))
	if err == nil {
		t.Fatal("ожидали отказ для файла больше лимита")
	}
	if !strings.Contains(err.Error(), "МБ") {
		t.Errorf("непонятное сообщение об ошибке: %v", err)
	}
}
