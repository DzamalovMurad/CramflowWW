package service

import (
	"testing"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

func testCart() []pricedItem {
	standard := &model.Product{ID: 1, Category: model.CategoryStandard}
	premium := &model.Product{ID: 2, Category: model.CategoryPremium}
	return []pricedItem{
		{item: model.OrderItem{Price: 2000, Quantity: 1}, product: standard},
		{item: model.OrderItem{Price: 3000, Quantity: 2}, product: premium},
	}
}

func TestEligibleAmount(t *testing.T) {
	cases := []struct {
		name      string
		appliesTo string
		want      int
	}{
		{"весь заказ", "all", 8000},
		{"пустая область = весь заказ", "", 8000},
		{"категория", "category:Премиум", 6000},
		{"категория без совпадений", "category:Люкс", 0},
		{"список товаров", "products:1", 2000},
		{"список товаров с пробелами", "products: 1, 2", 8000},
		{"список без совпадений", "products:99", 0},
	}
	for _, c := range cases {
		promo := &model.PromoCode{AppliesTo: c.appliesTo}
		if got := eligibleAmount(promo, testCart()); got != c.want {
			t.Errorf("%s: eligibleAmount = %d, ожидали %d", c.name, got, c.want)
		}
	}
}

func TestIsDeliverySlot(t *testing.T) {
	for _, slot := range DeliverySlots {
		if !isDeliverySlot(slot) {
			t.Errorf("слот %q должен быть валидным", slot)
		}
	}
	for _, bad := range []string{"", "09:00-11:00", "в течение часа", "10:00–12:00"} {
		if isDeliverySlot(bad) {
			t.Errorf("%q не должен быть валидным слотом", bad)
		}
	}
}

func TestSlotStart(t *testing.T) {
	start, err := slotStart("2026-08-01", "14:00-16:00")
	if err != nil {
		t.Fatalf("slotStart: %v", err)
	}
	if start.Hour() != 14 || start.Location() != MskLocation {
		t.Errorf("ожидали 14:00 MSK, получили %v", start)
	}
}
