package model

import (
	"strings"
	"testing"
	"time"
)

// ─── Конечный автомат статусов ─────────────────────────────────────────────

func TestNextStatus(t *testing.T) {
	want := map[string]string{
		StatusNew:        StatusConfirmed,
		StatusConfirmed:  StatusAssembling,
		StatusAssembling: StatusDelivering,
		StatusDelivering: StatusDelivered,
		StatusDelivered:  "", // терминальный
		StatusCancelled:  "", // терминальный
		"nonsense":       "",
	}
	for from, to := range want {
		if got := NextStatus(from); got != to {
			t.Errorf("NextStatus(%s) = %q, want %q", from, got, to)
		}
	}
}

func TestAllowedTransition(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusNew, StatusConfirmed, true},
		{StatusConfirmed, StatusAssembling, true},
		{StatusAssembling, StatusDelivering, true},
		{StatusDelivering, StatusDelivered, true},

		{StatusNew, StatusDelivered, false},  // прыжок через весь конвейер
		{StatusNew, StatusAssembling, false}, // прыжок через шаг
		{StatusDelivering, StatusNew, false}, // назад нельзя
		{StatusDelivered, StatusDelivering, false},

		{StatusNew, StatusCancelled, true}, // отмена из любого нетерминального
		{StatusDelivering, StatusCancelled, true},
		{StatusDelivered, StatusCancelled, false}, // доставленный не отменить
		{StatusCancelled, StatusCancelled, false},
		{StatusCancelled, StatusConfirmed, false},

		{StatusNew, "nonsense", false},
	}
	for _, c := range cases {
		if got := AllowedTransition(c.from, c.to); got != c.want {
			t.Errorf("AllowedTransition(%s → %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

// Конвейер обязан доходить от нового заказа до доставленного и не зацикливаться.
func TestPipelineReachesDelivered(t *testing.T) {
	s := StatusNew
	for i := 0; i < 10; i++ {
		next := NextStatus(s)
		if next == "" {
			break
		}
		if !AllowedTransition(s, next) {
			t.Fatalf("шаг %s → %s не разрешён автоматом", s, next)
		}
		s = next
	}
	if s != StatusDelivered {
		t.Fatalf("конвейер закончился на %q, ожидали %q", s, StatusDelivered)
	}
}

func TestEveryStatusHasLabel(t *testing.T) {
	for _, s := range StatusOrder {
		if StatusLabels[s] == "" {
			t.Errorf("нет подписи для статуса %q", s)
		}
	}
	if len(StatusOrder) != len(StatusLabels) {
		t.Errorf("StatusOrder (%d) и StatusLabels (%d) разошлись", len(StatusOrder), len(StatusLabels))
	}
}

func TestIsTerminal(t *testing.T) {
	for _, s := range []string{StatusDelivered, StatusCancelled} {
		if !IsTerminal(s) {
			t.Errorf("%s должен быть терминальным", s)
		}
	}
	for _, s := range ActiveStatuses {
		if IsTerminal(s) {
			t.Errorf("%s не должен быть терминальным", s)
		}
	}
}

// ─── Уведомления клиенту ───────────────────────────────────────────────────

// Регрессия: шаблон без %d давал в сообщении мусор «%!(EXTRA uint=4)».
func TestClientStatusTextNoFormatLeftovers(t *testing.T) {
	for status := range ClientStatusMessages {
		text, ok := ClientStatusText(status, 4)
		if !ok {
			t.Errorf("статус %s: ожидали текст", status)
			continue
		}
		if strings.Contains(text, "%!") || strings.Contains(text, "EXTRA") || strings.Contains(text, "%d") {
			t.Errorf("статус %s: испорченное форматирование: %q", status, text)
		}
		if !strings.Contains(text, "4") {
			t.Errorf("статус %s: нет номера заказа: %q", status, text)
		}
	}
}

func TestClientStatusTextUnknownStatus(t *testing.T) {
	if _, ok := ClientStatusText(StatusNew, 1); ok {
		t.Error("для статуса new клиенту писать не нужно: он только что оформил заказ")
	}
	if _, ok := ClientStatusText("nonsense", 1); ok {
		t.Error("неизвестный статус не должен давать текст")
	}
}

// Каждый статус, о котором пишем клиенту, обязан существовать в автомате.
func TestClientMessagesMatchStatuses(t *testing.T) {
	known := map[string]bool{}
	for _, s := range StatusOrder {
		known[s] = true
	}
	for s := range ClientStatusMessages {
		if !known[s] {
			t.Errorf("уведомление настроено для несуществующего статуса %q", s)
		}
	}
}

// ─── Деньги ────────────────────────────────────────────────────────────────

func TestApplyDiscount(t *testing.T) {
	cases := []struct {
		subtotal, percent int
		discount, total   int
	}{
		{10000, 10, 1000, 9000},
		{10000, 0, 0, 10000},
		{0, 15, 0, 0},
		{2990, 15, 448, 2542},  // округление вниз в пользу магазина
		{999, 10, 99, 900},     // 99.9 → 99
		{1, 50, 0, 1},          // скидка меньше рубля не даётся
		{10000, -5, 0, 10000},  // отрицательный процент игнорируется
		{10000, 150, 10000, 0}, // больше 100% не бывает
		{-100, 10, 0, 0},       // отрицательная сумма невозможна
	}
	for _, c := range cases {
		d, tot := ApplyDiscount(c.subtotal, c.percent)
		if d != c.discount || tot != c.total {
			t.Errorf("ApplyDiscount(%d, %d) = (%d, %d), want (%d, %d)",
				c.subtotal, c.percent, d, tot, c.discount, c.total)
		}
		// Инвариант, ради которого всё и затевалось.
		if d+tot != max(c.subtotal, 0) && c.subtotal > 0 {
			t.Errorf("ApplyDiscount(%d, %d): скидка+итог = %d, а сумма = %d",
				c.subtotal, c.percent, d+tot, c.subtotal)
		}
		if tot < 0 || d < 0 {
			t.Errorf("ApplyDiscount(%d, %d) дал отрицательное значение", c.subtotal, c.percent)
		}
	}
}

// ─── Промокоды ─────────────────────────────────────────────────────────────

func TestPromoUsable(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name string
		p    PromoCode
		want bool
	}{
		{"обычный активный", PromoCode{IsActive: true}, true},
		{"выключенный", PromoCode{IsActive: false}, false},
		{"истёкший", PromoCode{IsActive: true, ExpiresAt: &past}, false},
		{"ещё действует", PromoCode{IsActive: true, ExpiresAt: &future}, true},
		{"лимит выбран", PromoCode{IsActive: true, MaxUses: 5, Uses: 5}, false},
		{"лимит превышен", PromoCode{IsActive: true, MaxUses: 5, Uses: 9}, false},
		{"лимит не выбран", PromoCode{IsActive: true, MaxUses: 5, Uses: 4}, true},
		{"без лимита", PromoCode{IsActive: true, MaxUses: 0, Uses: 1000}, true},
	}
	for _, c := range cases {
		if got := c.p.Usable(now); got != c.want {
			t.Errorf("%s: Usable = %v, want %v", c.name, got, c.want)
		}
	}
}

// Скидка считается по правилу кода и никогда не уводит заказ в минус.
func TestPromoApply(t *testing.T) {
	cases := []struct {
		name         string
		p            PromoCode
		subtotal     int
		wantDiscount int
		wantTotal    int
	}{
		{"процент", PromoCode{DiscountType: DiscountTypePercent, DiscountValue: 15}, 4990, 748, 4242},
		{"процент округляется вниз", PromoCode{DiscountType: DiscountTypePercent, DiscountValue: 10}, 999, 99, 900},
		{"фиксированная", PromoCode{DiscountType: DiscountTypeFixed, DiscountValue: 500}, 4990, 500, 4490},
		{"фиксированная больше суммы", PromoCode{DiscountType: DiscountTypeFixed, DiscountValue: 5000}, 3000, 3000, 0},
		{"фиксированная равна сумме", PromoCode{DiscountType: DiscountTypeFixed, DiscountValue: 3000}, 3000, 3000, 0},
		{"пустая корзина", PromoCode{DiscountType: DiscountTypeFixed, DiscountValue: 500}, 0, 0, 0},
		// Пустой тип = процент: так лежат коды, заведённые до появления типов.
		{"тип не указан", PromoCode{DiscountValue: 20}, 1000, 200, 800},
	}
	for _, c := range cases {
		discount, total := c.p.Apply(c.subtotal)
		if discount != c.wantDiscount || total != c.wantTotal {
			t.Errorf("%s: Apply(%d) = %d/%d, ожидали %d/%d",
				c.name, c.subtotal, discount, total, c.wantDiscount, c.wantTotal)
		}
		if total < 0 || discount < 0 {
			t.Errorf("%s: отрицательные деньги %d/%d", c.name, discount, total)
		}
		if c.subtotal > 0 && discount+total != c.subtotal {
			t.Errorf("%s: скидка и итог не сходятся с суммой %d", c.name, c.subtotal)
		}
	}
}

func TestPromoMeetsMinimum(t *testing.T) {
	cases := []struct {
		name     string
		p        PromoCode
		subtotal int
		want     bool
	}{
		{"без порога", PromoCode{}, 100, true},
		{"ровно порог", PromoCode{MinOrderAmount: 3000}, 3000, true},
		{"выше порога", PromoCode{MinOrderAmount: 3000}, 3001, true},
		{"ниже порога", PromoCode{MinOrderAmount: 3000}, 2999, false},
	}
	for _, c := range cases {
		if got := c.p.MeetsMinimum(c.subtotal); got != c.want {
			t.Errorf("%s: MeetsMinimum(%d) = %v, want %v", c.name, c.subtotal, got, c.want)
		}
	}
}

// Percent показывает процент только у процентных кодов: витрина Mini App
// умеет рисовать «−15%», но не «−15 ₽».
func TestPromoPercentAndDescribe(t *testing.T) {
	percent := PromoCode{DiscountType: DiscountTypePercent, DiscountValue: 15}
	fixed := PromoCode{DiscountType: DiscountTypeFixed, DiscountValue: 500}

	if percent.Percent() != 15 || fixed.Percent() != 0 {
		t.Errorf("Percent: процентный %d, фиксированный %d", percent.Percent(), fixed.Percent())
	}
	if percent.Describe() != "15%" || fixed.Describe() != "500 ₽" {
		t.Errorf("Describe: %q и %q", percent.Describe(), fixed.Describe())
	}
}

// ─── Товары ────────────────────────────────────────────────────────────────

func TestProductAvailable(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		p    Product
		want bool
	}{
		{"обычный", Product{}, true},
		{"скрытый", Product{IsHidden: true}, false},
		{"архивный", Product{ArchivedAt: &now}, false},
		{"и то и другое", Product{IsHidden: true, ArchivedAt: &now}, false},
	}
	for _, c := range cases {
		if got := c.p.Available(); got != c.want {
			t.Errorf("%s: Available = %v, want %v", c.name, got, c.want)
		}
	}
}
