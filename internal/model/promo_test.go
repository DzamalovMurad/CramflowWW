package model

import (
	"testing"
	"time"
)

func TestPromoDiscount(t *testing.T) {
	cases := []struct {
		name     string
		promo    PromoCode
		eligible int
		want     int
	}{
		{"процент от суммы", PromoCode{Type: PromoPercent, Value: 10}, 5000, 500},
		{"процент округляется вниз в пользу клиента", PromoCode{Type: PromoPercent, Value: 15}, 999, 150},
		{"фикс меньше суммы", PromoCode{Type: PromoFixed, Value: 500}, 5000, 500},
		{"фикс не больше суммы", PromoCode{Type: PromoFixed, Value: 5000}, 2990, 2990},
		{"ноль подходящих товаров", PromoCode{Type: PromoPercent, Value: 50}, 0, 0},
	}
	for _, c := range cases {
		if got := c.promo.Discount(c.eligible); got != c.want {
			t.Errorf("%s: Discount(%d) = %d, ожидали %d", c.name, c.eligible, got, c.want)
		}
	}
}

func TestPromoDisplayUsable(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	cases := []struct {
		name  string
		promo PromoCode
		want  bool
	}{
		{"активный без ограничений", PromoCode{IsActive: true}, true},
		{"отключён", PromoCode{IsActive: false}, false},
		{"ещё не начался", PromoCode{IsActive: true, StartsAt: &future}, false},
		{"уже истёк", PromoCode{IsActive: true, ExpiresAt: &past}, false},
		{"в окне дат", PromoCode{IsActive: true, StartsAt: &past, ExpiresAt: &future}, true},
		{"лимит исчерпан", PromoCode{IsActive: true, MaxUses: 3, UsedCount: 3}, false},
		{"лимит не исчерпан", PromoCode{IsActive: true, MaxUses: 3, UsedCount: 2}, true},
	}
	for _, c := range cases {
		if got := c.promo.DisplayUsable(now); got != c.want {
			t.Errorf("%s: DisplayUsable = %v, ожидали %v", c.name, got, c.want)
		}
	}
}
