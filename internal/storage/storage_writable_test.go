package storage

import (
	"os"
	"path/filepath"
	"testing"
)

// Регрессия: том Railway монтируется от root, а сервис работает под другим
// пользователем — MkdirAll проходил, а первая же запись фото падала.
func TestLocalCheckWritable(t *testing.T) {
	dir := t.TempDir()
	local, err := NewLocal(dir, "/uploads")
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	if err := local.CheckWritable(); err != nil {
		t.Fatalf("каталог доступен для записи, а проверка ругается: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("после проверки остался мусор: %v", entries)
	}

	if os.Geteuid() == 0 {
		t.Skip("под root запись разрешена всегда — отказ не воспроизвести")
	}
	readonly := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(readonly, 0o555); err != nil {
		t.Fatalf("подготовка каталога: %v", err)
	}
	if err := (&Local{Dir: readonly, BaseURL: "/uploads"}).CheckWritable(); err == nil {
		t.Error("каталог без права записи должен давать ошибку")
	}
}
