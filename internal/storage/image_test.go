package storage

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// makeImage рисует картинку с градиентом и контрастными линиями —
// на однотонном фоне артефакты ресайза не проявились бы.
func makeImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.RGBA{uint8(x % 256), uint8(y % 256), 120, 255}
			if x%50 == 0 || y%50 == 0 {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	return buf.Bytes()
}

// Большой снимок с камеры уменьшается под витрину и заметно легчает.
func TestPrepareImageDownscalesLargePhoto(t *testing.T) {
	// Исходник как с телефона: JPEG высокого качества в полном разрешении.
	var srcBuf bytes.Buffer
	if err := jpeg.Encode(&srcBuf, makeImage(4032, 3024), &jpeg.Options{Quality: 97}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}
	src := srcBuf.Bytes()
	out, ext, err := PrepareImage(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("ext = %q, ожидали .jpg", ext)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("результат не читается как JPEG: %v", err)
	}
	if cfg.Width != maxImageSide {
		t.Errorf("ширина = %d, ожидали %d", cfg.Width, maxImageSide)
	}
	if want := 3024 * maxImageSide / 4032; cfg.Height != want {
		t.Errorf("высота = %d, ожидали %d (пропорции не сохранены)", cfg.Height, want)
	}
	if len(out) >= len(src) {
		t.Errorf("результат не легче исходника: %d ≥ %d", len(out), len(src))
	}
	// Витрина должна грузиться на мобильном интернете: держим бюджет на кадр.
	const budget = 1_500_000
	if len(out) > budget {
		t.Errorf("результат %d байт — тяжелее бюджета %d", len(out), budget)
	}
	t.Logf("%d×%d %.2f МБ → %d×%d %.2f МБ", 4032, 3024, float64(len(src))/1e6,
		cfg.Width, cfg.Height, float64(len(out))/1e6)
}

// Небольшой снимок не растягиваем, но перекодируем в JPEG.
func TestPrepareImageKeepsSmallPhotoSize(t *testing.T) {
	src := encodePNG(t, makeImage(800, 600))
	out, ext, err := PrepareImage(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("ext = %q, ожидали .jpg", ext)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("результат не читается как JPEG: %v", err)
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Errorf("размер изменился: %d×%d, ожидали 800×600", cfg.Width, cfg.Height)
	}
}

// Вертикальный кадр ограничивается по высоте.
func TestPrepareImagePortrait(t *testing.T) {
	src := encodePNG(t, makeImage(1500, 3000))
	out, _, err := PrepareImage(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out))
	if cfg.Height != maxImageSide {
		t.Errorf("высота = %d, ожидали %d", cfg.Height, maxImageSide)
	}
	if cfg.Width != 1500*maxImageSide/3000 {
		t.Errorf("ширина = %d, пропорции не сохранены", cfg.Width)
	}
}

// Неизвестный формат (например, HEIC) сохраняем как есть, а не теряем.
func TestPrepareImageUnknownFormatPassthrough(t *testing.T) {
	raw := []byte("не картинка, а какие-то байты")
	out, ext, err := PrepareImage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	if ext != "" {
		t.Errorf("ext = %q, ожидали пустое (расширение исходника сохраняется)", ext)
	}
	if !bytes.Equal(out, raw) {
		t.Error("данные неизвестного формата изменились")
	}
}
