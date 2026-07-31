package repository

import "testing"

func TestIsSeasonal(t *testing.T) {
	fresh := []string{"пионы", "тюльпаны", "эустома"}

	cases := []struct {
		name string
		want bool
		why  string
	}{
		{"Пионовидный микс", true, "общий корень «пион»"},
		{"Тюльпаны Пинк", true, "прямое совпадение"},
		{"Эустома белая", true, "прямое совпадение"},
		{"Розы Эквадор", false, "роз сегодня на базе нет"},
		{"Розовые каллы", false, "«розовые» — про цвет, не про розы"},
		{"Сердце из цветов", false, "состав неизвестен"},
	}
	for _, c := range cases {
		if got := IsSeasonal(c.name, fresh); got != c.want {
			t.Errorf("IsSeasonal(%q) = %v, ожидали %v (%s)", c.name, got, c.want, c.why)
		}
	}

	if IsSeasonal("Пионовидный микс", nil) {
		t.Error("без списка свежего бейджа быть не должно")
	}
}

func TestSharesStem(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"пионовидный", "пионы", true},
		{"розовые", "розы", false}, // общее только «роз» — три буквы
		{"мак", "мак", true},       // короткое слово: только точное совпадение
		{"мак", "маки", false},     // «мак» короче корня, равенства нет
		{"тюльпаны", "тюльпан", true},
	}
	for _, c := range cases {
		if got := sharesStem(c.a, c.b); got != c.want {
			t.Errorf("sharesStem(%q, %q) = %v, ожидали %v", c.a, c.b, got, c.want)
		}
	}
}
