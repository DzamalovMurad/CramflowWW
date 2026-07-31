package repository

import (
	"errors"
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// --- Настройки рантайма (key/value) ---

// GetSetting возвращает значение настройки или def, если её нет.
func (r *Repository) GetSetting(key, def string) string {
	var s model.Setting
	if err := r.DB.Where("key = ?", key).First(&s).Error; err != nil {
		return def
	}
	return s.Value
}

// SetSetting сохраняет настройку (upsert по ключу).
func (r *Repository) SetSetting(key, value string) error {
	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{"value": value, "updated_at": time.Now()}),
	}).Create(&model.Setting{Key: key, Value: value, UpdatedAt: time.Now()}).Error
}

// FallbackOrdersEnabled — включён ли заказ через диалог бота.
func (r *Repository) FallbackOrdersEnabled() bool {
	return r.GetSetting(model.SettingFallbackOrders, "off") == "on"
}

func (r *Repository) SetFallbackOrders(on bool) error {
	value := "off"
	if on {
		value = "on"
	}
	return r.SetSetting(model.SettingFallbackOrders, value)
}

// --- Подборки витрины ---

// TopHits — до limit хитов для пустой корзины и fallback-заказа в боте:
// сначала по sort_order, затем свежие. Если хитов нет вовсе, отдаём просто
// начало витрины — экран с призывом «посмотреть хиты» не должен быть пустым.
func (r *Repository) TopHits(limit int) ([]model.Product, error) {
	all, err := r.visibleProducts()
	if err != nil {
		return nil, err
	}
	hits := make([]model.Product, 0, limit)
	for _, p := range all {
		if p.IsHit && len(p.Variants) > 0 {
			hits = append(hits, p)
		}
	}
	if len(hits) == 0 {
		for _, p := range all {
			if len(p.Variants) > 0 {
				hits = append(hits, p)
			}
		}
	}
	// visibleProducts отдаёт свежие первыми — стабильная сортировка сохранит
	// этот порядок внутри одинакового sort_order.
	sortStable(hits, func(a, b model.Product) bool { return a.SortOrder < b.SortOrder })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

// sortStable — устойчивая сортировка вставками: список витрины короткий,
// заводить sort.SliceStable ради десятка элементов ни к чему.
func sortStable(items []model.Product, less func(a, b model.Product) bool) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && less(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func (r *Repository) SetProductLowStock(id uint, low bool) error {
	defer r.InvalidateCatalog()
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("low_stock", low).Error
}

func (r *Repository) SetProductSortOrder(id uint, order int) error {
	defer r.InvalidateCatalog()
	return r.DB.Model(&model.Product{}).Where("id = ?", id).Update("sort_order", order).Error
}

// --- Сезонность ---

// SeasonalNames — список позиций из «сегодня на базе» за дату.
// Бейдж «сезонный» не хранится у товара: он выводится из того, что флорист
// закупил утром (/fresh), поэтому пропадает сам, когда позиция уходит из списка.
func (r *Repository) SeasonalNames(date string) []string {
	fresh, err := r.GetFreshToday(date)
	if err != nil || fresh.Items == "" {
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return nil
	}
	parts := strings.FieldsFunc(fresh.Items, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// seasonalStem — сколько первых букв считаем общим корнем.
// Четыре — рабочий компромисс: «пионы» цепляют «Пионовидный микс»
// (корень «пион»), но «розы» не цепляют «Розовые каллы» (общее только «роз»),
// а это разные цветы и бейдж там был бы враньём.
const seasonalStem = 4

// IsSeasonal — попадает ли товар в сегодняшний список свежего.
//
// Здесь нужен не поиск с опечатками (MatchesSearch), а совпадение по корню:
// флорист пишет «пионы», товар называется «Пионовидный микс». Поэтому слова
// сравниваются по общему началу.
func IsSeasonal(name string, seasonal []string) bool {
	words := splitWords(normalizeRu(name))
	for _, item := range seasonal {
		for _, itemWord := range splitWords(normalizeRu(item)) {
			for _, w := range words {
				if sharesStem(w, itemWord) {
					return true
				}
			}
		}
	}
	return false
}

func splitWords(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}

// sharesStem — у слов общее начало длиной seasonalStem.
// Слова короче считаем совпавшими только при полном равенстве.
func sharesStem(a, b string) bool {
	ar, br := []rune(a), []rune(b)
	if len(ar) < seasonalStem || len(br) < seasonalStem {
		return a == b
	}
	return string(ar[:seasonalStem]) == string(br[:seasonalStem])
}
