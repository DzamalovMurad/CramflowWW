package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/config"
	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/testdb"
)

type apiHarness struct {
	api  *API
	srv  http.Handler
	repo *repository.Repository
	svc  *service.Service
	cfg  *config.Config
}

func newAPIHarness(t *testing.T) *apiHarness {
	t.Helper()
	db := testdb.Open(t)

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("нет базы часовых поясов: %v", err)
	}
	cfg := &config.Config{
		Location: loc, ShopOpenHour: 0, ShopCloseHour: 23,
		BotToken: fakeToken, InitDataTTL: 24 * time.Hour,
		WebDist: t.TempDir(), UploadDir: t.TempDir(),
	}
	log := slog.New(slog.DiscardHandler)
	repo := repository.New(db, cfg.Now)
	svc := service.New(repo, cfg, log)
	api := &API{Repo: repo, Service: svc, Cfg: cfg, Log: log}
	t.Cleanup(api.Close)

	return &apiHarness{api: api, srv: api.Routes(), repo: repo, svc: svc, cfg: cfg}
}

// do выполняет запрос от имени пользователя tgID (0 — без initData).
func (h *apiHarness) do(t *testing.T, method, path string, body string, tgID int64) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	// Уникальный адрес на запрос: rate limiter не должен мешать тестам.
	r.RemoteAddr = "127.0.0.1:1"
	r.Header.Set("X-Forwarded-For", uniqueIP())

	if tgID != 0 {
		fields := map[string]string{
			"user":      `{"id":` + strconv.FormatInt(tgID, 10) + `,"first_name":"Клиент"}`,
			"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
		}
		r.Header.Set("X-Telegram-Init-Data", signInitData(fakeToken, fields))
	}
	rec := httptest.NewRecorder()
	h.srv.ServeHTTP(rec, r)
	return rec
}

// uniqueIP — свой адрес на каждый запрос, чтобы rate limiter не мешал тестам.
var ipCounter atomic.Int64

func uniqueIP() string {
	n := ipCounter.Add(1)
	return fmt.Sprintf("10.%d.%d.%d", n/65536%256, n/256%256, n%256)
}

// seedOrder создаёт заказ от имени клиента tgID и возвращает его id.
func (h *apiHarness) seedOrder(t *testing.T, tgID int64) uint {
	t.Helper()
	p := &model.Product{
		Name: "Тестовый букет", Category: model.CategoryStandard,
		Variants: []model.ProductVariant{{Quantity: 9, Price: 3000}},
	}
	if err := h.repo.CreateProduct(t.Context(), p); err != nil {
		t.Fatalf("товар: %v", err)
	}
	o, err := h.svc.CreateOrder(t.Context(), service.OrderInput{
		Items:           []service.OrderItemInput{{VariantID: p.Variants[0].ID, Quantity: 1}},
		Name:            "Секретное Имя",
		Phone:           "+79001234567",
		DeliveryAddress: "Москва, секретный адрес, 1",
		DeliveryDate:    time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
		DeliveryTime:    "к 15:00",
		TelegramID:      tgID,
	})
	if err != nil {
		t.Fatalf("заказ: %v", err)
	}
	return o.ID
}

// ─── Приватность заказов ───────────────────────────────────────────────────

// Регрессия на утечку персональных данных: раньше запрос без initData
// получал tgID = 0, все «гостевые» заказы висели на пользователе 0,
// и перебором id из браузера читались чужие имя, телефон и адрес.
func TestIntegrationOrderIsPrivate(t *testing.T) {
	h := newAPIHarness(t)
	const owner, stranger = int64(1111), int64(2222)
	orderID := h.seedOrder(t, owner)
	path := "/api/orders/" + strconv.FormatUint(uint64(orderID), 10)

	t.Run("владелец видит свой заказ", func(t *testing.T) {
		rec := h.do(t, http.MethodGet, path, "", owner)
		if rec.Code != http.StatusOK {
			t.Fatalf("код %d, ожидали 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "секретный адрес") {
			t.Error("владельцу не отдали его собственный заказ")
		}
	})

	t.Run("посторонний не видит чужой заказ", func(t *testing.T) {
		rec := h.do(t, http.MethodGet, path, "", stranger)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("код %d, ожидали 404", rec.Code)
		}
		assertNoPII(t, rec.Body.String())
	})

	t.Run("без initData доступа нет", func(t *testing.T) {
		rec := h.do(t, http.MethodGet, path, "", 0)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("код %d, ожидали 401", rec.Code)
		}
		assertNoPII(t, rec.Body.String())
	})

	t.Run("с подделанной initData доступа нет", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("X-Forwarded-For", uniqueIP())
		r.Header.Set("X-Telegram-Init-Data",
			"user=%7B%22id%22%3A1111%7D&auth_date=99999999999&hash=deadbeef")
		rec := httptest.NewRecorder()
		h.srv.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("код %d, ожидали 401", rec.Code)
		}
		assertNoPII(t, rec.Body.String())
	})
}

func assertNoPII(t *testing.T, body string) {
	t.Helper()
	for _, secret := range []string{"Секретное Имя", "+79001234567", "секретный адрес"} {
		if strings.Contains(body, secret) {
			t.Errorf("в ответе утекли персональные данные: %q", secret)
		}
	}
}

// Список «мои заказы» отдаёт только заказы запрашивающего.
func TestIntegrationMyOrdersIsolated(t *testing.T) {
	h := newAPIHarness(t)
	const a, b = int64(3333), int64(4444)
	h.seedOrder(t, a)

	rec := h.do(t, http.MethodGet, "/api/my/orders", "", b)
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d", rec.Code)
	}
	var orders []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &orders); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if len(orders) != 0 {
		t.Fatalf("чужому клиенту отдали %d заказов", len(orders))
	}

	own := h.do(t, http.MethodGet, "/api/my/orders", "", a)
	if err := json.Unmarshal(own.Body.Bytes(), &orders); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("владельцу отдали %d заказов, ожидали 1", len(orders))
	}
}

// Заказ нельзя оформить без валидной initData.
func TestIntegrationCreateOrderRequiresAuth(t *testing.T) {
	h := newAPIHarness(t)
	body := `{"items":[{"variant_id":1,"quantity":1}],"name":"Аноним","phone":"+79000000000",
	          "delivery_address":"Москва, ул. Ленина, 1","delivery_date":"2030-01-01","delivery_time":"к 15:00"}`

	rec := h.do(t, http.MethodPost, "/api/orders", body, 0)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("код %d, ожидали 401", rec.Code)
	}
}

// ─── Идемпотентность на уровне HTTP ────────────────────────────────────────

func TestIntegrationCreateOrderIdempotentOverHTTP(t *testing.T) {
	h := newAPIHarness(t)
	p := &model.Product{
		Name: "Букет", Category: model.CategoryStandard,
		Variants: []model.ProductVariant{{Quantity: 9, Price: 2500}},
	}
	if err := h.repo.CreateProduct(t.Context(), p); err != nil {
		t.Fatalf("товар: %v", err)
	}

	body := `{"items":[{"variant_id":` + strconv.FormatUint(uint64(p.Variants[0].ID), 10) + `,"quantity":1}],` +
		`"name":"Иван","phone":"+79000000000",` +
		`"delivery_address":"Москва, ул. Ленина, 1","delivery_date":"` +
		time.Now().AddDate(0, 0, 1).Format("2006-01-02") + `","delivery_time":"к 15:00"}`

	send := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "/api/orders", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Idempotency-Key", "checkout-1")
		r.Header.Set("X-Forwarded-For", uniqueIP())
		r.Header.Set("X-Telegram-Init-Data", signInitData(fakeToken, map[string]string{
			"user":      `{"id":5555,"first_name":"Иван"}`,
			"auth_date": strconv.FormatInt(time.Now().Unix(), 10),
		}))
		rec := httptest.NewRecorder()
		h.srv.ServeHTTP(rec, r)
		return rec
	}

	first, second := send(), send()
	if first.Code != http.StatusCreated {
		t.Fatalf("первый запрос: %d — %s", first.Code, first.Body.String())
	}
	if second.Code != http.StatusCreated {
		t.Fatalf("повтор: %d — %s", second.Code, second.Body.String())
	}

	var a, b struct {
		ID uint `json:"id"`
	}
	_ = json.Unmarshal(first.Body.Bytes(), &a)
	_ = json.Unmarshal(second.Body.Bytes(), &b)
	if a.ID != b.ID {
		t.Fatalf("двойная отправка создала заказы #%d и #%d", a.ID, b.ID)
	}
}

// ─── Прочие эндпоинты ──────────────────────────────────────────────────────

func TestIntegrationHealthReflectsDatabase(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(t, http.MethodGet, "/api/health", "", 0)
	if rec.Code != http.StatusOK {
		t.Fatalf("здоровый сервис вернул %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"db":"up"`) {
		t.Errorf("health не отражает состояние БД: %s", rec.Body.String())
	}

	// Закрываем пул — health обязан стать красным, а не врать «ok».
	sqlDB, err := h.repo.DB.DB()
	if err != nil {
		t.Fatalf("sql.DB: %v", err)
	}
	_ = sqlDB.Close()

	down := h.do(t, http.MethodGet, "/api/health", "", 0)
	if down.Code != http.StatusServiceUnavailable {
		t.Fatalf("при мёртвой БД health вернул %d, ожидали 503", down.Code)
	}
}

func TestIntegrationUnknownAPIPathIs404(t *testing.T) {
	h := newAPIHarness(t)
	for _, path := range []string{"/api/nope", "/api/orders", "/telegram/secret-guess"} {
		rec := h.do(t, http.MethodGet, path, "", 0)
		if rec.Code == http.StatusOK {
			t.Errorf("%s вернул 200 вместо 404 — служебный путь провалился в SPA", path)
		}
	}
}

func TestIntegrationConfigEndpoint(t *testing.T) {
	h := newAPIHarness(t)
	rec := h.do(t, http.MethodGet, "/api/config", "", 0)
	if rec.Code != http.StatusOK {
		t.Fatalf("код %d", rec.Code)
	}
	var cfg struct {
		Today            string `json:"today"`
		OpenHour         int    `json:"open_hour"`
		CloseHour        int    `json:"close_hour"`
		ExpressAvailable bool   `json:"express_available"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("разбор: %v", err)
	}
	if cfg.Today != h.cfg.Today() {
		t.Errorf("today = %q, ожидали %q", cfg.Today, h.cfg.Today())
	}
	if cfg.CloseHour != 23 {
		t.Errorf("close_hour = %d", cfg.CloseHour)
	}
}

// Контракт /api/promo: старое поле discount_percent остаётся на месте (на нём
// держится корзина Mini App), а сумму скидки считает сервер, а не клиент.
func TestIntegrationPromoEndpointContract(t *testing.T) {
	h := newAPIHarness(t)
	if err := h.repo.CreatePromo(t.Context(), &model.PromoCode{
		Code: "SPRING15", DiscountType: model.DiscountTypePercent, DiscountValue: 15, IsActive: true,
	}); err != nil {
		t.Fatalf("создание промокода: %v", err)
	}
	if err := h.repo.CreatePromo(t.Context(), &model.PromoCode{
		Code: "MINUS500", DiscountType: model.DiscountTypeFixed, DiscountValue: 500,
		MinOrderAmount: 3000, IsActive: true,
	}); err != nil {
		t.Fatalf("создание промокода: %v", err)
	}

	var percent struct {
		Code            string `json:"code"`
		DiscountPercent int    `json:"discount_percent"`
		DiscountType    string `json:"discount_type"`
		DiscountAmount  int    `json:"discount_amount"`
		Total           int    `json:"total"`
	}
	rec := h.do(t, http.MethodGet, "/api/promo/spring15?subtotal=4990", "", 900001)
	if rec.Code != http.StatusOK {
		t.Fatalf("процентный код: HTTP %d, тело %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &percent); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if percent.Code != "SPRING15" || percent.DiscountPercent != 15 {
		t.Errorf("контракт витрины сломан: %+v", percent)
	}
	if percent.DiscountAmount != 748 || percent.Total != 4242 {
		t.Errorf("скидка посчитана неверно: %d/%d, ожидали 748/4242",
			percent.DiscountAmount, percent.Total)
	}

	// Фиксированная скидка: процента у неё нет, зато есть тип и сумма.
	rec = h.do(t, http.MethodGet, "/api/promo/MINUS500?subtotal=4990", "", 900001)
	if rec.Code != http.StatusOK {
		t.Fatalf("фиксированный код: HTTP %d, тело %s", rec.Code, rec.Body.String())
	}
	var fixed struct {
		DiscountPercent int    `json:"discount_percent"`
		DiscountType    string `json:"discount_type"`
		DiscountValue   int    `json:"discount_value"`
		MinOrderAmount  int    `json:"min_order_amount"`
		DiscountAmount  int    `json:"discount_amount"`
		Total           int    `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &fixed); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if fixed.DiscountPercent != 0 || fixed.DiscountType != model.DiscountTypeFixed ||
		fixed.DiscountValue != 500 || fixed.MinOrderAmount != 3000 {
		t.Errorf("описание фиксированной скидки: %+v", fixed)
	}
	if fixed.DiscountAmount != 500 || fixed.Total != 4490 {
		t.Errorf("скидка посчитана неверно: %d/%d, ожидали 500/4490",
			fixed.DiscountAmount, fixed.Total)
	}

	// Порог не выполнен — код не отдаётся вовсе.
	rec = h.do(t, http.MethodGet, "/api/promo/MINUS500?subtotal=2000", "", 900001)
	if rec.Code != http.StatusNotFound {
		t.Errorf("код с невыполненным порогом: HTTP %d, ожидали 404", rec.Code)
	}
}

// Публичные эндпоинты обязаны иметь лимит: перебор промокодов должен упираться в стену.
func TestIntegrationPromoEndpointIsRateLimited(t *testing.T) {
	h := newAPIHarness(t)
	blocked := false
	for i := 0; i < 40; i++ {
		r := httptest.NewRequest(http.MethodGet, "/api/promo/GUESS"+strconv.Itoa(i), nil)
		r.Header.Set("X-Forwarded-For", "203.0.113.77") // один и тот же клиент
		rec := httptest.NewRecorder()
		h.srv.ServeHTTP(rec, r)
		if rec.Code == http.StatusTooManyRequests {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("перебор промокодов не был ограничен")
	}
}

// Тело запроса, снятое из работающего Mini App (Chromium, форма заполнена
// вручную). Сервер обязан принимать ровно его: рассинхрон имён полей или
// формата даты между TypeScript и Go собирается без ошибок, а ломается
// только в бою.
func TestIntegrationServerAcceptsRealFrontendPayload(t *testing.T) {
	h := newAPIHarness(t)
	p := &model.Product{
		Name: "Розы Эквадор", Category: model.CategoryPremium,
		Variants: []model.ProductVariant{{Quantity: 9, Price: 2990}},
	}
	if err := h.repo.CreateProduct(t.Context(), p); err != nil {
		t.Fatalf("товар: %v", err)
	}

	body := `{
	 "items": [{"variant_id": ` + strconv.FormatUint(uint64(p.Variants[0].ID), 10) + `, "quantity": 1}],
	 "name": "Мария Тестова",
	 "phone": "+7 900 111-22-33",
	 "delivery_address": "Москва, Тверская 1, кв 5",
	 "delivery_date": "` + h.cfg.Today() + `",
	 "delivery_time": "в течение часа",
	 "recipient_name": "Мама",
	 "recipient_phone": "+7 900 999-88-77",
	 "comment": "",
	 "card_text": "С днём рождения!",
	 "is_anonymous": false,
	 "promo_code": ""
	}`

	rec := h.do(t, http.MethodPost, "/api/orders", body, 606060)
	if rec.Code != http.StatusCreated {
		t.Fatalf("сервер отклонил тело, которое шлёт Mini App: %d — %s", rec.Code, rec.Body.String())
	}

	var got orderView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("ответ не разбирается: %v", err)
	}
	if got.TotalPrice != 2990 {
		t.Errorf("итог = %d, ожидали 2990", got.TotalPrice)
	}
	if len(got.Items) != 1 || got.Items[0].FlowersCount != 9 {
		t.Errorf("позиции ответа неполны: %+v", got.Items)
	}
	// Поля, без которых экран подтверждения и «мои заказы» покажут пустоту.
	if got.StatusLabel == "" || got.DeliveryDate == "" || got.DeliveryTime == "" {
		t.Errorf("ответ без обязательных для интерфейса полей: %+v", got)
	}
	if got.Items[0].ProductID == 0 || got.Items[0].VariantID == 0 {
		t.Error("в ответе нет product_id/variant_id — «повторить заказ» работать не будет")
	}

	// Получатель и открытка должны доехать до карточки админа.
	full, err := h.repo.GetOrder(t.Context(), got.ID)
	if err != nil {
		t.Fatalf("чтение заказа: %v", err)
	}
	if full.RecipientName != "Мама" || full.RecipientPhone != "+79009998877" {
		t.Errorf("получатель сохранён неверно: %q / %q", full.RecipientName, full.RecipientPhone)
	}
	if full.CardText != "С днём рождения!" {
		t.Errorf("текст открытки: %q", full.CardText)
	}
	if full.User.Phone != "+79001112233" {
		t.Errorf("телефон заказчика не нормализован: %q", full.User.Phone)
	}
}
