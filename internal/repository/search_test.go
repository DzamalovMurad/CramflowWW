package repository

import "testing"

func TestMatchesSearch(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  bool
	}{
		// Точное и подстрочное
		{"Хризантемы кустовые", "хризантемы", true},
		{"Хризантемы кустовые", "куст", true},
		// Окончания
		{"Хризантемы кустовые", "хризантема", true},
		{"Хризантемы кустовые", "хризантем", true},
		{"Розы Эквадор", "роза", true},
		// Опечатки
		{"Хризантемы кустовые", "хрезантемы", true},
		{"Хризантемы кустовые", "хризонтемы", true},
		{"Тюльпаны Пинк", "тюльпан", true},
		{"Тюльпаны Пинк", "тульпаны", true},
		// ё/е и регистр
		{"Пионовидный микс", "ПИОН", true},
		// Несколько токенов: все должны совпасть
		{"Розы Эквадор", "розы эквадор", true},
		{"Розы Эквадор", "розы кения", false},
		// Не должно находить чужое
		{"Розы Эквадор", "пионы", false},
		{"Гвоздики белые", "розы", false},
		// Пустой запрос — всё подходит
		{"Что угодно", "", true},
	}
	for _, c := range cases {
		if got := MatchesSearch(c.name, c.query); got != c.want {
			t.Errorf("MatchesSearch(%q, %q) = %v, want %v", c.name, c.query, got, c.want)
		}
	}
}

func TestLevenshtein(t *testing.T) {
	if d := levenshtein([]rune("роза"), []rune("розы")); d != 1 {
		t.Errorf("lev(роза, розы) = %d, want 1", d)
	}
	if d := levenshtein([]rune(""), []rune("abc")); d != 3 {
		t.Errorf("lev('', abc) = %d, want 3", d)
	}
}
