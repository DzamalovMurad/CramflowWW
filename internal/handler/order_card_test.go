package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

func metroOrder() *model.Order {
	return &model.Order{
		ID:           7,
		DeliveryType: model.DeliveryMetro,
		MetroStation: "Сокольники",
		DeliveryDate: "2026-08-20",
		DeliveryTime: "в течение часа",
		TotalPrice:   2990,
		Status:       model.StatusNew,
		User:         model.User{Name: "Мурад", Phone: "+79990000000"},
	}
}

func addressOrder() *model.Order {
	return &model.Order{
		ID:              8,
		DeliveryType:    model.DeliveryAddress,
		DeliveryAddress: "ул. Мира, 1, кв. 5",
		DeliveryDate:    "2026-08-20",
		TotalPrice:      4490,
		Status:          model.StatusNew,
		User:            model.User{Name: "Аня", Phone: "+79991111111"},
	}
}

// Способ доставки — первое, что видит человек, собирающий заказ:
// он должен стоять сразу под номером заказа, а не в конце карточки.
func TestFormatOrderDeliveryFirst(t *testing.T) {
	cases := map[string]struct {
		order *model.Order
		want  string
	}{
		"метро": {metroOrder(), "🚇 Метро: Сокольники"},
		"адрес": {addressOrder(), "📍 По адресу: ул. Мира, 1, кв. 5"},
	}
	for name, c := range cases {
		lines := strings.Split(formatOrder(c.order, true), "\n")
		if len(lines) < 2 || lines[1] != c.want {
			t.Errorf("%s: вторая строка карточки %q, ожидали %q", name, lines[1], c.want)
		}
	}
}

func TestFormatOrderApprovalMarker(t *testing.T) {
	const marker = "⚠️ Согласовать доставку с клиентом"

	if card := formatOrder(addressOrder(), true); !strings.Contains(card, marker) {
		t.Error("на несогласованном заказе по адресу нет маркера")
	}
	if card := formatOrder(metroOrder(), true); strings.Contains(card, marker) {
		t.Error("у доставки до метро согласовывать нечего — маркера быть не должно")
	}

	agreed := addressOrder()
	at := time.Date(2026, 8, 12, 14, 20, 0, 0, time.UTC)
	agreed.DeliveryAgreedAt = &at
	agreed.DeliveryAgreedBy = 12345
	card := formatOrder(agreed, false)
	if strings.Contains(card, marker) {
		t.Error("после согласования маркер должен исчезнуть")
	}
	if !strings.Contains(card, "12.08 14:20") || !strings.Contains(card, "12345") {
		t.Errorf("не видно, кто и когда согласовал доставку: %q", card)
	}
}

// Регрессия: в карточке не должно появиться никакой «стоимости доставки» —
// её в системе нет, итог заказа это всегда только букеты.
func TestFormatOrderHasNoDeliveryFee(t *testing.T) {
	for _, o := range []*model.Order{metroOrder(), addressOrder()} {
		card := formatOrder(o, true)
		for _, forbidden := range []string{"Доставка:", "Стоимость доставки", "за доставку"} {
			if strings.Contains(card, forbidden) {
				t.Errorf("в карточке появилась строка о цене доставки (%q): %q", forbidden, card)
			}
		}
		if !strings.Contains(card, "Итого за букеты:") {
			t.Errorf("итог должен быть подписан как «за букеты»: %q", card)
		}
	}
}

func TestAdminOrderKeyboardApprovalButton(t *testing.T) {
	has := func(o *model.Order) bool {
		for _, row := range adminOrderKeyboard(o).InlineKeyboard {
			for _, btn := range row {
				if btn.CallbackData != nil && strings.HasPrefix(*btn.CallbackData, "dok:") {
					return true
				}
			}
		}
		return false
	}
	if !has(addressOrder()) {
		t.Error("на заказе по адресу нет кнопки «Доставка согласована»")
	}
	if has(metroOrder()) {
		t.Error("на заказе до метро кнопка согласования не нужна")
	}
	done := addressOrder()
	at := time.Now()
	done.DeliveryAgreedAt = &at
	if has(done) {
		t.Error("после согласования кнопка должна пропасть")
	}
}

func TestDeliveryTimeLabel(t *testing.T) {
	if got := deliveryTimeLabel(metroOrder()); got != "в течение часа" {
		t.Errorf("выбранное время должно показываться как есть, получили %q", got)
	}
	metroNoTime := metroOrder()
	metroNoTime.DeliveryTime = ""
	if got := deliveryTimeLabel(metroNoTime); got != "время не указано" {
		t.Errorf("метро без времени: %q", got)
	}
	if got := deliveryTimeLabel(addressOrder()); got != "время согласует менеджер" {
		t.Errorf("адрес без времени: %q", got)
	}
}
