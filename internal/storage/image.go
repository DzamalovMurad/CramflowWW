package storage

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png" // регистрируем декодер PNG
	"io"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // регистрируем декодер WebP
)

// Параметры подготовки фото для витрины. Исходник приходит файлом (оригинал
// с камеры, 5–15 МБ) — отдавать его в Mini App нельзя: каталог грузился бы
// десятки секунд. Один качественный ресайз даёт резкую картинку в ~0.5 МБ:
// 2000 px с запасом покрывает любой телефон, включая 3x-retina.
const (
	maxImageSide = 2000
	jpegQuality  = 92
)

// PrepareImage декодирует снимок, при необходимости уменьшает его
// высококачественной интерполяцией и перекодирует в JPEG.
// Форматы, которые Go не умеет читать (например, HEIC с iPhone),
// возвращаются без изменений — лучше сохранить как есть, чем потерять фото.
func PrepareImage(r io.Reader) (data []byte, ext string, err error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}

	src, _, decErr := image.Decode(bytes.NewReader(raw))
	if decErr != nil {
		return raw, "", nil // формат неизвестен — кладём оригинал
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxImageSide && h <= maxImageSide {
		// Уменьшать нечего, но перекодируем в JPEG: снимок мог прийти
		// 20-мегабайтным PNG, а на витрине это лишний вес.
		return encodeJPEG(src)
	}

	if w >= h {
		h = h * maxImageSide / w
		w = maxImageSide
	} else {
		w = w * maxImageSide / h
		h = maxImageSide
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// CatmullRom — резкий результат без ступенек, в отличие от билинейного.
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return encodeJPEG(dst)
}

func encodeJPEG(img image.Image) ([]byte, string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), ".jpg", nil
}
