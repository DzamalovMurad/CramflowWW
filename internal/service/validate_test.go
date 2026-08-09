package service

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/config"
)

// testService — сервис с фиксированными часами магазина и фиксированным «сейчас».
// Валидация зависит от времени, поэтому тесты не должны зависеть от того,
// когда их запустили.
func testService(t *testing.T, now time.Time) *Service {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("нет базы часовых поясов: %v", err)
	}
	cfg := &config.Config{
		Location:      loc,
		ShopOpenHour:  9,
		ShopCloseHour: 21,
		InitDataTTL:   24 * time.Hour,
	}
	s := New(nil, cfg, slog.New(slog.DiscardHandler))
	s.nowFn = func() time.Time { return now.In(loc) }
	return s
}

func mskTime(t *testing.T, s string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("нет базы часовых поясов: %v", err)
	}
	v, err := time.ParseInLocation("2006-01-02 15:04", s, loc)
	if err != nil {
		t.Fatalf("некорректное время в тесте %q: %v", s, err)
	}
	return v
}

// ─── Телефон ───────────────────────────────────────────────────────────────

func TestNormalizePhone(t *testing.T) {
	ok := map[string]string{
		"+7 900 000-00-00":  "+79000000000",
		"89000000000":       "+79000000000",
		"79000000000":       "+79000000000",
		"9000000000":        "+79000000000",
		"+7(900)000 00 00":  "+79000000000",
		"8 (900) 000-00-00": "+79000000000",
		"+380 44 000 0000":  "+380440000000", // не только Россия
	}
	for in, want := range ok {
		got, err := normalizePhone(in)
		if err != nil {
			t.Errorf("normalizePhone(%q) вернул ошибку: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizePhone(%q) = %q, want %q", in, got, want)
		}
	}

	bad := []string{"", "12345", "телефон", "+7", "1234567890123456789", "---"}
	for _, in := range bad {
		if got, err := normalizePhone(in); err == nil {
			t.Errorf("normalizePhone(%q) = %q, ожидали ошибку", in, got)
		}
	}
}

// ─── Окно доставки ─────────────────────────────────────────────────────────

func TestValidateDeliveryExpress(t *testing.T) {
	// Полдень рабочего дня — экспресс доступен.
	s := testService(t, mskTime(t, "2026-08-01 12:00"))
	if err := s.validateDelivery("2026-08-01", ExpressDelivery); err != nil {
		t.Errorf("экспресс в 12:00 должен приниматься: %v", err)
	}
	// На завтра экспресса быть не может.
	if err := s.validateDelivery("2026-08-02", ExpressDelivery); err == nil {
		t.Error("экспресс на завтра должен отклоняться")
	}
	// Ночью магазин закрыт.
	night := testService(t, mskTime(t, "2026-08-01 03:00"))
	if err := night.validateDelivery("2026-08-01", ExpressDelivery); err == nil {
		t.Error("экспресс в 03:00 должен отклоняться")
	}
}

func TestValidateDeliveryAtTime(t *testing.T) {
	s := testService(t, mskTime(t, "2026-08-01 12:00"))

	good := []string{"к 14:00", "к 21:00", "к 13:00"}
	for _, v := range good {
		if err := s.validateDelivery("2026-08-01", v); err != nil {
			t.Errorf("«%s» на сегодня должно приниматься: %v", v, err)
		}
	}

	bad := map[string]string{
		"к 12:30":            "меньше часа на сборку",
		"к 08:00":            "до открытия",
		"к 22:00":            "после закрытия",
		"14:00":              "неверный формат",
		"":                   "пусто",
		"в течение получаса": "несуществующий режим",
	}
	for v, why := range bad {
		if err := s.validateDelivery("2026-08-01", v); err == nil {
			t.Errorf("«%s» должно отклоняться (%s)", v, why)
		}
	}

	// На завтра запас в час не нужен — принимаем любое время в окне.
	if err := s.validateDelivery("2026-08-02", "к 09:00"); err != nil {
		t.Errorf("«к 09:00» на завтра должно приниматься: %v", err)
	}
}

func TestValidateDeliveryDate(t *testing.T) {
	s := testService(t, mskTime(t, "2026-08-01 12:00"))

	if err := s.validateDelivery("2026-07-31", "к 14:00"); err == nil {
		t.Error("прошедшая дата должна отклоняться")
	}
	if err := s.validateDelivery("2027-08-01", "к 14:00"); err == nil {
		t.Error("дата за горизонтом предзаказа должна отклоняться")
	}
	for _, bad := range []string{"", "завтра", "01.08.2026", "2026-13-45", "2026-08-01T00:00:00Z"} {
		if err := s.validateDelivery(bad, "к 14:00"); err == nil {
			t.Errorf("дата %q должна отклоняться", bad)
		}
	}
}

// Поздний вечер: сегодня уже никак — но завтра можно.
func TestValidateDeliveryLateEvening(t *testing.T) {
	s := testService(t, mskTime(t, "2026-08-01 20:45"))
	if err := s.validateDelivery("2026-08-01", "к 21:00"); err == nil {
		t.Error("за 15 минут до закрытия заказ ко времени должен отклоняться")
	}
	if err := s.validateDelivery("2026-08-02", "к 10:00"); err != nil {
		t.Errorf("заказ на завтра вечером должен приниматься: %v", err)
	}
}

// ─── Валидация формы заказа ────────────────────────────────────────────────

func validOrder() OrderInput {
	return OrderInput{
		Items:           []OrderItemInput{{VariantID: 1, Quantity: 2}},
		Name:            "Иван",
		Phone:           "+79000000000",
		DeliveryAddress: "Москва, ул. Тверская, 1, кв. 5",
		DeliveryDate:    "2026-08-01",
		DeliveryTime:    "к 15:00",
		TelegramID:      777,
	}
}

func TestValidateAcceptsGoodOrder(t *testing.T) {
	s := testService(t, mskTime(t, "2026-08-01 12:00"))
	out, err := s.validate(validOrder())
	if err != nil {
		t.Fatalf("корректный заказ отклонён: %v", err)
	}
	if out.Phone != "+79000000000" {
		t.Errorf("телефон не нормализован: %q", out.Phone)
	}
}

func TestValidateRejectsBadOrders(t *testing.T) {
	s := testService(t, mskTime(t, "2026-08-01 12:00"))

	cases := map[string]func(*OrderInput){
		"пустая корзина":        func(o *OrderInput) { o.Items = nil },
		"нулевое количество":    func(o *OrderInput) { o.Items[0].Quantity = 0 },
		"отрицательное кол-во":  func(o *OrderInput) { o.Items[0].Quantity = -5 },
		"количество за лимитом": func(o *OrderInput) { o.Items[0].Quantity = 1000 },
		"нулевой вариант":       func(o *OrderInput) { o.Items[0].VariantID = 0 },
		"дубль позиции": func(o *OrderInput) {
			o.Items = append(o.Items, OrderItemInput{VariantID: 1, Quantity: 1})
		},
		"слишком много позиций": func(o *OrderInput) {
			o.Items = nil
			for i := 1; i <= maxCartLines+1; i++ {
				o.Items = append(o.Items, OrderItemInput{VariantID: uint(i), Quantity: 1})
			}
		},
		"пустое имя":              func(o *OrderInput) { o.Name = "  " },
		"имя из 1 буквы":          func(o *OrderInput) { o.Name = "И" },
		"длинное имя":             func(o *OrderInput) { o.Name = strings.Repeat("и", maxNameLen+1) },
		"нет телефона":            func(o *OrderInput) { o.Phone = "" },
		"мусорный телефон":        func(o *OrderInput) { o.Phone = "позвоните мне" },
		"пустой адрес":            func(o *OrderInput) { o.DeliveryAddress = "" },
		"короткий адрес":          func(o *OrderInput) { o.DeliveryAddress = "дом" },
		"длинный адрес":           func(o *OrderInput) { o.DeliveryAddress = strings.Repeat("а", maxAddressLen+1) },
		"длинный комментарий":     func(o *OrderInput) { o.Comment = strings.Repeat("а", maxCommentLen+1) },
		"длинная открытка":        func(o *OrderInput) { o.CardText = strings.Repeat("а", maxCardTextLen+1) },
		"длинный промокод":        func(o *OrderInput) { o.PromoCode = strings.Repeat("A", maxPromoCodeLen+1) },
		"длинный ключ":            func(o *OrderInput) { o.IdempotencyKey = strings.Repeat("x", 65) },
		"плохое время":            func(o *OrderInput) { o.DeliveryTime = "когда-нибудь" },
		"плохая дата":             func(o *OrderInput) { o.DeliveryDate = "вчера" },
		"получатель без телефона": func(o *OrderInput) { o.RecipientName = "Мама" },
		"получатель с мусорным телефоном": func(o *OrderInput) {
			o.RecipientName, o.RecipientPhone = "Мама", "нет"
		},
	}

	for name, mutate := range cases {
		in := validOrder()
		mutate(&in)
		if _, err := s.validate(in); err == nil {
			t.Errorf("%s: ожидали ошибку валидации", name)
		}
	}
}

// Пользовательский ввод не должен уезжать в БД с хвостами и двойными пробелами.
func TestValidateNormalizesWhitespace(t *testing.T) {
	s := testService(t, mskTime(t, "2026-08-01 12:00"))
	in := validOrder()
	in.Name = "  Иван   Петров  "
	in.DeliveryAddress = "  Москва,   Тверская   1  "
	in.PromoCode = " welcome10 "

	out, err := s.validate(in)
	if err != nil {
		t.Fatalf("валидация: %v", err)
	}
	if out.Name != "Иван Петров" {
		t.Errorf("имя = %q", out.Name)
	}
	if out.DeliveryAddress != "Москва, Тверская 1" {
		t.Errorf("адрес = %q", out.DeliveryAddress)
	}
	if out.PromoCode != "WELCOME10" {
		t.Errorf("промокод = %q", out.PromoCode)
	}
}

// Заказ без Telegram-пользователя не должен создаваться в принципе:
// иначе PII разных людей сливаются в одну «гостевую» запись.
func TestCreateOrderRequiresTelegramUser(t *testing.T) {
	s := testService(t, mskTime(t, "2026-08-01 12:00"))
	in := validOrder()
	in.TelegramID = 0
	if _, err := s.CreateOrder(t.Context(), in); err == nil {
		t.Fatal("заказ без telegram_id должен отклоняться")
	}
}
