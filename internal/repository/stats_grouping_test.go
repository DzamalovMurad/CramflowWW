package repository

import (
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

func TestGroupSmallSources(t *testing.T) {
	// 100 заказов: instagram 60, direct 30, vk 5, yandex 3, avito 2.
	// При пороге 5% в отчёт попадают только instagram, direct и vk (ровно 5%),
	// остальные сворачиваются в «другое».
	stats := []SourceStat{
		{Source: "instagram", Orders: 60, Revenue: 600},
		{Source: "direct", Orders: 30, Revenue: 300},
		{Source: "vk", Orders: 5, Revenue: 50},
		{Source: "yandex", Orders: 3, Revenue: 30},
		{Source: "avito", Orders: 2, Revenue: 20},
	}

	got := GroupSmallSources(stats, 4, 0.05)
	if len(got) != 4 {
		t.Fatalf("строк отчёта = %d, ожидалось 4 (три канала + «другое»): %+v", len(got), got)
	}
	last := got[len(got)-1]
	if last.Source != model.SourceOther {
		t.Errorf("последняя строка = %q, ожидалось %q", last.Source, model.SourceOther)
	}
	if last.Orders != 5 || last.Revenue != 50 {
		t.Errorf("«другое» = %d заказов на %d, ожидалось 5 на 50", last.Orders, last.Revenue)
	}
}

func TestGroupSmallSourcesKeepsLimit(t *testing.T) {
	// Каналов больше, чем помещается в отчёт: хвост уходит в «другое»,
	// даже если каждый из них крупнее порога.
	var stats []SourceStat
	for i := 0; i < 8; i++ {
		stats = append(stats, SourceStat{Source: "src", Orders: 10, Revenue: 100})
	}
	got := GroupSmallSources(stats, 4, 0.05)
	if len(got) != 5 {
		t.Fatalf("строк = %d, ожидалось 5 (4 канала + «другое»)", len(got))
	}
	if got[4].Orders != 40 {
		t.Errorf("«другое» = %d заказов, ожидалось 40", got[4].Orders)
	}
}

func TestGroupSmallSourcesEmpty(t *testing.T) {
	if got := GroupSmallSources(nil, 4, 0.05); got != nil {
		t.Errorf("пустая статистика дала %+v, ожидался nil", got)
	}
}

func TestPeriodStart(t *testing.T) {
	now := time.Date(2026, 7, 31, 15, 30, 0, 0, time.UTC)

	// «Сегодня» — с полуночи, а не «24 часа назад».
	today := PeriodStart(PeriodToday, now)
	if today.Hour() != 0 || today.Minute() != 0 || today.Day() != 31 {
		t.Errorf("начало «сегодня» = %v, ожидалась полночь 31-го", today)
	}

	if week := PeriodStart(PeriodWeek, now); week != now.AddDate(0, 0, -7) {
		t.Errorf("начало недели = %v", week)
	}
	if month := PeriodStart(PeriodMonth, now); month != now.AddDate(0, 0, -30) {
		t.Errorf("начало месяца = %v", month)
	}
}
