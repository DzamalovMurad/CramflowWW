package model

import (
	"strings"
	"testing"
)

func TestClientOrderStatusTextGift(t *testing.T) {
	o := &Order{ID: 7, Status: StatusDelivering, CardText: "с днём рождения"}
	text, ok := ClientOrderStatusText(o)
	if !ok || !strings.Contains(text, "сюрприз") {
		t.Errorf("подарочная доставка должна давать подарочный текст, получили %q", text)
	}

	plain := &Order{ID: 7, Status: StatusDelivering}
	text, ok = ClientOrderStatusText(plain)
	if !ok || strings.Contains(text, "сюрприз") {
		t.Errorf("обычная доставка не должна давать подарочный текст, получили %q", text)
	}
}

func TestClientOrderStatusTextCancelReason(t *testing.T) {
	o := &Order{ID: 3, Status: StatusCancelled, CancelReason: "клиент передумал"}
	text, ok := ClientOrderStatusText(o)
	if !ok || !strings.Contains(text, "клиент передумал") {
		t.Errorf("отмена с причиной должна включать причину, получили %q", text)
	}
	if strings.Contains(text, "%") {
		t.Errorf("мусор форматирования: %q", text)
	}
}

func TestPhotoCaption(t *testing.T) {
	for _, typ := range []string{PhotoAssembled, PhotoDelivered} {
		text, ok := PhotoCaption(typ, 12)
		if !ok || !strings.Contains(text, "12") || strings.Contains(text, "%") {
			t.Errorf("тип %s: некорректная подпись %q", typ, text)
		}
	}
	if _, ok := PhotoCaption("nonsense", 1); ok {
		t.Error("неизвестный тип фото не должен давать подпись")
	}
}

func TestPhotoTypeForStatus(t *testing.T) {
	if PhotoTypeForStatus(StatusAssembling) != PhotoAssembled {
		t.Error("на сборке фото должно быть типа assembled")
	}
	if PhotoTypeForStatus(StatusDelivering) != PhotoDelivered {
		t.Error("в доставке фото должно быть типа delivered")
	}
	if PhotoTypeForStatus(StatusDelivered) != PhotoDelivered {
		t.Error("после доставки фото должно быть типа delivered")
	}
}

func TestSourceFromStartParam(t *testing.T) {
	cases := map[string]string{
		"product_5": "product_5",
		"src_insta": "insta",
		"promo2024": "promo2024",
		"  src_x  ": "x",
		"":          "",
	}
	for in, want := range cases {
		if got := SourceFromStartParam(in); got != want {
			t.Errorf("SourceFromStartParam(%q) = %q, ждали %q", in, got, want)
		}
	}
	long := strings.Repeat("ф", 100)
	if got := SourceFromStartParam(long); len([]rune(got)) != 64 {
		t.Errorf("длинный параметр должен обрезаться до 64 символов, получили %d", len([]rune(got)))
	}
}
