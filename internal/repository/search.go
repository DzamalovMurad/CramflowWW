package repository

import (
	"strings"
	"unicode"
)

// normalizeRu приводит строку к нижнему регистру и склеивает ё/е.
func normalizeRu(s string) string {
	return strings.ReplaceAll(strings.ToLower(s), "ё", "е")
}

// MatchesSearch — нечёткое совпадение названия с запросом.
// Каждый токен запроса должен найтись в названии: как подстрока,
// по общему корню (учёт окончаний: «хризантем» ~ «хризантемы»)
// или с 1–2 опечатками (расстояние Левенштейна).
func MatchesSearch(name, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	name = normalizeRu(name)
	words := strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})

	for _, tok := range strings.Fields(normalizeRu(query)) {
		if strings.Contains(name, tok) {
			continue
		}
		found := false
		for _, w := range words {
			if fuzzyWordMatch(w, tok) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func fuzzyWordMatch(word, tok string) bool {
	wr, tr := []rune(word), []rune(tok)

	// Общий корень: допускаем расхождение в окончании («роза» ~ «розы»,
	// «хризантема» ~ «хризантемы» ~ «хризантем»).
	if len(wr) >= 4 && len(tr) >= 4 {
		pre := min(len(wr), len(tr)) - 2
		if pre >= 4 {
			pre = min(pre, len(wr), len(tr))
			if string(wr[:pre]) == string(tr[:pre]) {
				return true
			}
		}
	}

	// Опечатки: 1 для коротких слов, 2 для длинных.
	tol := 1
	if len(tr) > 5 {
		tol = 2
	}
	return levenshtein(wr, tr) <= tol
}

// levenshtein — классическое редакционное расстояние (две строки DP-таблицы).
func levenshtein(a, b []rune) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
