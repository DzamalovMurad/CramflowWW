package model

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeMetroStation(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"Сокольники", "Сокольники", true},
		{"сокольники", "Сокольники", true},
		{"  ТЕПЛЫЙ СТАН ", "Тёплый Стан", true}, // регистр, пробелы и е вместо ё
		{"китай город", "Китай-город", true},    // дефис клиенты часто теряют
		{"Воробьевы горы", "Воробьёвы горы", true},
		{"Невский проспект", "", false}, // не Москва
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := NormalizeMetroStation(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizeMetroStation(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

// Список станций должен быть без дублей: он идёт в выпадашку checkout как есть.
func TestMetroStationsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range MetroStations {
		key := normalizeStation(s)
		if seen[key] {
			t.Errorf("станция %q встречается дважды", s)
		}
		seen[key] = true
		if strings.TrimSpace(s) != s || s == "" {
			t.Errorf("некорректное название станции: %q", s)
		}
	}
	if len(MetroStations) < 200 {
		t.Errorf("в списке всего %d станций — похоже, он неполный", len(MetroStations))
	}
}

func TestDeliveryText(t *testing.T) {
	metro := &Order{DeliveryType: DeliveryMetro, MetroStation: "Сокольники"}
	if got := metro.DeliveryText(); got != "Доставка до метро Сокольники — бесплатно" {
		t.Errorf("метро: %q", got)
	}
	addr := &Order{DeliveryType: DeliveryAddress, DeliveryAddress: "ул. Мира, 1"}
	if got := addr.DeliveryText(); !strings.Contains(got, "менеджер свяжется") {
		t.Errorf("адрес: %q", got)
	}
	// Ни в одном тексте о доставке не должно быть суммы: стоимости курьера
	// в системе нет, её называет менеджер.
	for _, o := range []*Order{metro, addr} {
		if strings.Contains(o.DeliveryText(), "₽") {
			t.Errorf("в тексте о доставке появилась цена: %q", o.DeliveryText())
		}
	}
}

func TestNeedsDeliveryApproval(t *testing.T) {
	agreed := time.Now()
	cases := []struct {
		name string
		o    Order
		want bool
	}{
		{"адрес, не согласовано", Order{DeliveryType: DeliveryAddress, Status: StatusNew}, true},
		{"адрес, согласовано", Order{DeliveryType: DeliveryAddress, Status: StatusNew, DeliveryAgreedAt: &agreed}, false},
		{"адрес, доставлен", Order{DeliveryType: DeliveryAddress, Status: StatusDelivered}, false},
		{"адрес, отменён", Order{DeliveryType: DeliveryAddress, Status: StatusCancelled}, false},
		{"метро", Order{DeliveryType: DeliveryMetro, Status: StatusNew}, false},
	}
	for _, c := range cases {
		if got := c.o.NeedsDeliveryApproval(); got != c.want {
			t.Errorf("%s: получили %v, ожидали %v", c.name, got, c.want)
		}
	}
}

func TestValidDeliveryType(t *testing.T) {
	for _, t2 := range DeliveryTypes {
		if !ValidDeliveryType(t2) {
			t.Errorf("%s должен быть валидным", t2)
		}
	}
	for _, bad := range []string{"", "pickup", "shop", "METRO"} {
		if ValidDeliveryType(bad) {
			t.Errorf("%q не должен приниматься", bad)
		}
	}
}
