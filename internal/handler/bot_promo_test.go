package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/config"
	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Админ вводит акцию с телефона: «15%» и «500» должны пониматься одинаково
// уверенно, а мусор — отклоняться до похода в базу.
func TestParsePromoDiscount(t *testing.T) {
	cases := []struct {
		in        string
		wantType  string
		wantValue int
		wantErr   bool
	}{
		{"15%", model.DiscountTypePercent, 15, false},
		{" 15 % ", model.DiscountTypePercent, 15, false},
		{"500", model.DiscountTypeFixed, 500, false},
		{"500₽", model.DiscountTypeFixed, 500, false},
		{"1 000", model.DiscountTypeFixed, 1000, false},
		{"0", "", 0, true},
		{"-10", "", 0, true},
		{"много", "", 0, true},
		{"", "", 0, true},
		{"15%%", "", 0, true},
	}
	for _, c := range cases {
		typ, value, err := parsePromoDiscount(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: ожидали ошибку, получили %s/%d", c.in, typ, value)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: неожиданная ошибка %v", c.in, err)
			continue
		}
		if typ != c.wantType || value != c.wantValue {
			t.Errorf("%q: %s/%d, ожидали %s/%d", c.in, typ, value, c.wantType, c.wantValue)
		}
	}
}

func TestParsePromoLimits(t *testing.T) {
	cases := []struct {
		in                string
		wantMax, wantUser int
		wantErr           bool
	}{
		{"100 1", 100, 1, false},
		{"100", 100, 1, false}, // без второго числа — один раз на клиента
		{"-", 0, 1, false},     // без общего лимита, но всё равно один на клиента
		{"0 0", 0, 0, false},   // явное «без ограничений»
		{"100 1 1", 0, 0, true},
		{"сто", 0, 0, true},
		{"-5", 0, 0, true},
	}
	for _, c := range cases {
		maxUses, perUser, err := parsePromoLimits(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("%q: ожидали ошибку, получили %d/%d", c.in, maxUses, perUser)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: неожиданная ошибка %v", c.in, err)
			continue
		}
		if maxUses != c.wantMax || perUser != c.wantUser {
			t.Errorf("%q: %d/%d, ожидали %d/%d", c.in, maxUses, perUser, c.wantMax, c.wantUser)
		}
	}
}

// «До 31.12» для админа означает «включая 31 декабря», а не «до полуночи 31-го».
func TestParsePromoExpiry(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("нет базы часовых поясов: %v", err)
	}
	b := &Bot{cfg: &config.Config{Location: loc}}

	got, err := b.parsePromoExpiry("31.12.2026")
	if err != nil {
		t.Fatalf("разбор даты: %v", err)
	}
	if got == nil {
		t.Fatal("дата не разобрана")
	}
	endOfDay := time.Date(2026, 12, 31, 23, 59, 59, 0, loc)
	if !got.Equal(endOfDay) {
		t.Errorf("срок = %s, ожидали конец дня %s", got, endOfDay)
	}
	// Заказ, оформленный 31 декабря днём, ещё попадает в акцию.
	if !got.After(time.Date(2026, 12, 31, 18, 0, 0, 0, loc)) {
		t.Error("акция «до 31.12» должна действовать весь день 31 декабря")
	}

	if got, err := b.parsePromoExpiry("-"); err != nil || got != nil {
		t.Errorf("«-» = бессрочно, получили %v, %v", got, err)
	}
	if _, err := b.parsePromoExpiry("когда-нибудь"); err == nil {
		t.Error("непонятная дата должна отклоняться")
	}
}

// Код в заказе берётся из снимка: акции может уже не быть.
func TestOrderPromoCode(t *testing.T) {
	promo := &model.PromoCode{Code: "LIVE"}
	cases := []struct {
		name string
		o    model.Order
		want string
	}{
		{"снимок и акция", model.Order{AppliedPromoCode: "SNAP", PromoCode: promo}, "SNAP"},
		{"только акция", model.Order{PromoCode: promo}, "LIVE"},
		{"только снимок", model.Order{AppliedPromoCode: "SNAP"}, "SNAP"},
		{"без промокода", model.Order{}, ""},
	}
	for _, c := range cases {
		if got := orderPromoCode(&c.o); got != c.want {
			t.Errorf("%s: %q, ожидали %q", c.name, got, c.want)
		}
	}
}

// В списке акций админ должен видеть правило скидки, порог и остаток лимита.
func TestPromoSummary(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	p := &model.PromoCode{
		Code: "SPRING15", DiscountType: model.DiscountTypePercent, DiscountValue: 15,
		MinOrderAmount: 3000, MaxUses: 100, Uses: 7, PerUserLimit: 1, IsActive: true,
	}
	got := promoSummary(p, now)
	for _, want := range []string{"SPRING15", "15%", "3000", "7/100", "1 на клиента"} {
		if !strings.Contains(got, want) {
			t.Errorf("в описании %q нет %q", got, want)
		}
	}

	off := &model.PromoCode{Code: "OFF", DiscountType: model.DiscountTypeFixed, DiscountValue: 500}
	got = promoSummary(off, now)
	if !strings.Contains(got, "500 ₽") || !strings.Contains(got, "выключен") {
		t.Errorf("выключенный фиксированный код описан как %q", got)
	}
}
