package handler

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

func sampleRows() []repository.ExportRow {
	return []repository.ExportRow{{
		ID:              7,
		CreatedAt:       time.Date(2026, 7, 30, 12, 5, 0, 0, time.UTC),
		DeliveryDate:    "2026-07-31",
		DeliveryTime:    "к 15:30",
		Status:          model.StatusDelivered,
		Items:           "Розы Эквадор (9 шт) x1",
		ItemsTotal:      2990,
		TotalPrice:      2691,
		PromoCode:       "WELCOME10",
		PromoPercent:    10,
		Source:          model.SourceInstagram,
		ClientName:      "Мурад",
		ClientPhone:     "+79000000000",
		RecipientName:   "Мария",
		RecipientPhone:  "+79111111111",
		DeliveryAddress: "ул. Цветочная, 1",
		IsAnonymous:     true,
	}}
}

// Excel в русской локали открывает CSV только с BOM — без него кириллица
// превращается в «Ð Ð¾Ð·Ñ‹». Это главное требование к файлу.
func TestBuildOrdersCSVHasBOM(t *testing.T) {
	data, err := buildOrdersCSV(sampleRows())
	if err != nil {
		t.Fatalf("сборка CSV: %v", err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("файл начинается не с UTF-8 BOM — Excel испортит кириллицу")
	}
	if !bytes.Contains(data, []byte("Розы Эквадор")) {
		t.Error("состав заказа не попал в файл")
	}
}

func TestBuildOrdersCSVColumns(t *testing.T) {
	data, err := buildOrdersCSV(sampleRows())
	if err != nil {
		t.Fatalf("сборка CSV: %v", err)
	}

	body := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	r := csv.NewReader(bytes.NewReader(body))
	r.Comma = ';'
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("чтение CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("строк = %d, ожидалось 2 (заголовок + заказ)", len(records))
	}
	if len(records[0]) != len(csvHeader) {
		t.Errorf("колонок в заголовке = %d, ожидалось %d", len(records[0]), len(csvHeader))
	}
	if len(records[1]) != len(csvHeader) {
		t.Fatalf("колонок в строке = %d, ожидалось %d", len(records[1]), len(csvHeader))
	}

	row := records[1]
	if row[0] != "7" {
		t.Errorf("номер заказа = %q", row[0])
	}
	// Суммы — голые числа: с пробелами и «₽» Excel считает колонку текстом
	// и не даёт её просуммировать.
	if row[7] != "2691" {
		t.Errorf("итог = %q, ожидалось 2691 без форматирования", row[7])
	}
	if strings.Contains(row[7], "₽") || strings.Contains(row[7], " ") {
		t.Errorf("итог %q отформатирован — Excel не сможет сложить колонку", row[7])
	}
	if row[4] != model.StatusLabels[model.StatusDelivered] {
		t.Errorf("статус = %q, ожидалась русская подпись", row[4])
	}
	if row[13] != "Мария" || row[14] != "+79111111111" {
		t.Errorf("получатель = %q / %q", row[13], row[14])
	}
	if row[16] != "да" {
		t.Errorf("анонимность = %q, ожидалось «да»", row[16])
	}
}

// Разделитель полей и переводы строк внутри значений не должны ломать разбор.
func TestBuildOrdersCSVEscaping(t *testing.T) {
	rows := sampleRows()
	rows[0].DeliveryAddress = "ул. Цветочная, 1; кв. 5\nподъезд 2"
	rows[0].CancelReason = `клиент сказал "передумал"`

	data, err := buildOrdersCSV(rows)
	if err != nil {
		t.Fatalf("сборка CSV: %v", err)
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})))
	r.Comma = ';'
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("CSV не разбирается после экранирования: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("строк = %d, ожидалось 2 — перевод строки разорвал запись", len(records))
	}
	if records[1][15] != "ул. Цветочная, 1; кв. 5\nподъезд 2" {
		t.Errorf("адрес после разбора = %q", records[1][15])
	}
	if records[1][17] != `клиент сказал "передумал"` {
		t.Errorf("причина отмены после разбора = %q", records[1][17])
	}
}

func TestExportPeriodStart(t *testing.T) {
	now := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	if got := exportPeriodStart(exportWeek, now); got != now.AddDate(0, 0, -7) {
		t.Errorf("неделя = %v", got)
	}
	// «Всё» — нулевое время: выгружаются заказы за всю историю.
	if got := exportPeriodStart(exportAll, now); !got.IsZero() {
		t.Errorf("период «всё» = %v, ожидалось нулевое время", got)
	}
}
