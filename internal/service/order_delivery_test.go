package service

import (
	"testing"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// baseInput — минимально заполненный заказ; способ доставки дописывают тесты.
func baseInput() OrderInput {
	return OrderInput{
		Items:        []OrderItemInput{{VariantID: 1, Quantity: 1}},
		Name:         "Мурад",
		Phone:        "+79990000000",
		DeliveryDate: "2026-08-20",
	}
}

func wantErr(t *testing.T, in OrderInput, msg string) {
	t.Helper()
	_, err := validateOrderInput(in)
	if err == nil {
		t.Fatalf("ожидали ошибку %q, но заказ прошёл валидацию", msg)
	}
	if err.Error() != msg {
		t.Errorf("ошибка %q, ожидали %q", err.Error(), msg)
	}
}

func TestValidateRequiresDeliveryType(t *testing.T) {
	in := baseInput()
	in.DeliveryAddress = "ул. Мира, 1"
	wantErr(t, in, "выберите способ доставки")

	in.DeliveryType = "pickup" // самовывоза не существует
	wantErr(t, in, "выберите способ доставки")
}

func TestValidateMetroNeedsKnownStation(t *testing.T) {
	in := baseInput()
	in.DeliveryType = model.DeliveryMetro
	for _, station := range []string{"", "Улица Пушкина", "Невский проспект"} {
		in.MetroStation = station
		wantErr(t, in, "выберите станцию метро из списка")
	}
}

// При доставке до метро адрес не нужен, а станция сохраняется в каноничном виде.
func TestValidateMetroClearsAddress(t *testing.T) {
	in := baseInput()
	in.DeliveryType = model.DeliveryMetro
	in.MetroStation = "теплый стан"
	in.DeliveryAddress = "ул. Мира, 1"

	out, err := validateOrderInput(in)
	if err != nil {
		t.Fatalf("заказ до метро без адреса должен проходить: %v", err)
	}
	if out.MetroStation != "Тёплый Стан" {
		t.Errorf("станция не приведена к каноничному виду: %q", out.MetroStation)
	}
	if out.DeliveryAddress != "" {
		t.Errorf("адрес должен быть очищен, получили %q", out.DeliveryAddress)
	}
}

// При доставке по адресу адрес обязателен, а станция метро в заказ не попадает.
func TestValidateAddressNeedsAddressAndClearsStation(t *testing.T) {
	in := baseInput()
	in.DeliveryType = model.DeliveryAddress
	in.MetroStation = "Сокольники"
	wantErr(t, in, "укажите адрес доставки")

	in.DeliveryAddress = "  ул. Мира, 1  "
	out, err := validateOrderInput(in)
	if err != nil {
		t.Fatalf("заказ по адресу должен проходить: %v", err)
	}
	if out.DeliveryAddress != "ул. Мира, 1" {
		t.Errorf("адрес не обрезан: %q", out.DeliveryAddress)
	}
	if out.MetroStation != "" {
		t.Errorf("станция должна быть очищена, получили %q", out.MetroStation)
	}
}

func TestValidateDeliveryTimeOptional(t *testing.T) {
	in := baseInput()
	in.DeliveryType = model.DeliveryMetro
	in.MetroStation = "Сокольники"

	in.DeliveryTime = "" // время не выбрано — это допустимо
	if _, err := validateOrderInput(in); err != nil {
		t.Errorf("пустое время должно приниматься: %v", err)
	}

	in.DeliveryTime = "к 15:30"
	if _, err := validateOrderInput(in); err != nil {
		t.Errorf("время в окне должно приниматься: %v", err)
	}

	in.DeliveryTime = "к 03:00" // вне окна 9:00–21:00
	wantErr(t, in, "выберите время доставки (с 9:00 до 21:00)")
}
