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
	raw := encodePNG(t, makeImage(4032, 3024)) // типичный кадр с телефона

	data, ext, err := PrepareImage(raw, ".png")
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("расширение = %q, ожидали .jpg", ext)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("результат не читается как JPEG: %v", err)
	}
	if cfg.Width != maxImageSide {
		t.Errorf("ширина = %d, ожидали %d", cfg.Width, maxImageSide)
	}
	// Пропорции сохранены: 4032×3024 → 2000×1500.
	if cfg.Height != 1500 {
		t.Errorf("высота = %d, ожидали 1500 (пропорции не сохранены)", cfg.Height)
	}
	// Витрина должна грузиться на мобильном интернете: даже синтетический
	// «шум» — худший случай для JPEG — обязан уложиться в разумный вес.
	const maxCatalogPhotoBytes = 2 << 20
	if len(data) > maxCatalogPhotoBytes {
		t.Errorf("фото весит %d байт — слишком тяжело для витрины", len(data))
	}
}

// Вертикальный снимок ограничивается по высоте, а не по ширине.
func TestPrepareImageDownscalesPortrait(t *testing.T) {
	raw := encodePNG(t, makeImage(1500, 3000))

	data, _, err := PrepareImage(raw, ".png")
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("JPEG: %v", err)
	}
	if cfg.Height != maxImageSide || cfg.Width != 1000 {
		t.Errorf("размер = %d×%d, ожидали 1000×%d", cfg.Width, cfg.Height, maxImageSide)
	}
}

// Небольшой PNG не ресайзится, но перекодируется в JPEG: 20-мегабайтный PNG
// на витрине — лишний вес.
func TestPrepareImageRecodesSmallPNG(t *testing.T) {
	raw := encodePNG(t, makeImage(800, 600))

	data, ext, err := PrepareImage(raw, ".png")
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("расширение = %q", ext)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("JPEG: %v", err)
	}
	if cfg.Width != 800 || cfg.Height != 600 {
		t.Errorf("размер изменился: %d×%d", cfg.Width, cfg.Height)
	}
}

// Неизвестный формат (например, HEIC с iPhone) сохраняется как есть:
// лучше положить оригинал, чем потерять фото.
func TestPrepareImageKeepsUnknownFormat(t *testing.T) {
	raw := []byte("это не картинка, а какой-то HEIC")

	data, ext, err := PrepareImage(raw, ".heic")
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	if !bytes.Equal(data, raw) {
		t.Error("неизвестный формат должен сохраняться без изменений")
	}
	if ext != ".heic" {
		t.Errorf("расширение = %q, ожидали .heic", ext)
	}
}

// Файл без расширения не должен оставаться безымянным.
func TestPrepareImageFallsBackToJPGExtension(t *testing.T) {
	_, ext, err := PrepareImage([]byte("мусор"), "")
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	if ext != ".jpg" {
		t.Errorf("расширение = %q, ожидали .jpg", ext)
	}
}

// Ресайз не должен превращать картинку в кашу: контрастные линии остаются.
func TestPrepareImageKeepsDetail(t *testing.T) {
	raw := encodePNG(t, makeImage(3000, 3000))

	data, _, err := PrepareImage(raw, ".png")
	if err != nil {
		t.Fatalf("PrepareImage: %v", err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("JPEG: %v", err)
	}
	// Считаем «яркие» пиксели: белые линии сетки обязаны пережить уменьшение.
	bright := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 3 {
		for x := b.Min.X; x < b.Max.X; x += 3 {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r > 0xE000 && g > 0xE000 && bl > 0xE000 {
				bright++
			}
		}
	}
	if bright == 0 {
		t.Error("после ресайза не осталось контрастных деталей — картинка размылась")
	}
}
