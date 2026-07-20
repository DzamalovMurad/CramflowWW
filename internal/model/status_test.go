package model

import "testing"

func TestNextStatus(t *testing.T) {
	want := map[string]string{
		StatusNew:        StatusConfirmed,
		StatusConfirmed:  StatusAssembling,
		StatusAssembling: StatusPhotoSent,
		StatusPhotoSent:  StatusDelivering,
		StatusDelivering: StatusDelivered,
		StatusDelivered:  "", // терминальный
		StatusCancelled:  "", // терминальный
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
		{StatusNew, StatusDelivered, false},   // прыжок через конвейер
		{StatusNew, StatusAssembling, false},  // прыжок через шаг
		{StatusDelivering, StatusNew, false},  // назад нельзя
		{StatusNew, StatusCancelled, true},    // отмена из любого нетерминального
		{StatusDelivering, StatusCancelled, true},
		{StatusDelivered, StatusCancelled, false}, // доставленный не отменить
		{StatusCancelled, StatusCancelled, false},
		{StatusCancelled, StatusConfirmed, false},
	}
	for _, c := range cases {
		if got := AllowedTransition(c.from, c.to); got != c.want {
			t.Errorf("AllowedTransition(%s → %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
