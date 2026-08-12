package model

import (
	"testing"
	"time"
)

func TestProductFresh(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	soon := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	cases := map[string]struct {
		until *time.Time
		want  bool
	}{
		"срок не задан": {nil, false},
		"срок впереди":  {&soon, true},
		"срок истёк":    {&past, false},
		"ровно сейчас":  {&now, false}, // граница: «до» не включает сам момент
	}
	for name, c := range cases {
		p := Product{FreshUntil: c.until}
		if got := p.Fresh(now); got != c.want {
			t.Errorf("%s: получили %v, ожидали %v", name, got, c.want)
		}
	}
}

func TestProductDailyPick(t *testing.T) {
	today, yesterday := "2026-09-01", "2026-08-31"
	cases := map[string]struct {
		on   *string
		want bool
	}{
		"не назначен": {nil, false},
		"на сегодня":  {&today, true},
		"вчерашний":   {&yesterday, false}, // гаснет сам, без уборки
	}
	for name, c := range cases {
		p := Product{DailyPickOn: c.on}
		if got := p.DailyPick(today); got != c.want {
			t.Errorf("%s: получили %v, ожидали %v", name, got, c.want)
		}
	}
}

// StampBadges — единственное место, где витринные признаки попадают в JSON.
func TestStampBadges(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	until := now.Add(time.Hour)
	today := "2026-09-01"

	p := Product{FreshUntil: &until, DailyPickOn: &today}
	p.StampBadges(now, today)
	if !p.IsFresh || !p.IsDailyPick {
		t.Errorf("признаки не проставлены: fresh=%v daily=%v", p.IsFresh, p.IsDailyPick)
	}

	empty := Product{}
	empty.StampBadges(now, today)
	if empty.IsFresh || empty.IsDailyPick {
		t.Error("у товара без пометок признаки должны остаться выключенными")
	}
}
