package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/config"
	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

// requestTimeout — потолок на обработку одного HTTP-запроса.
// Отменяет и SQL: репозиторий работает через context.
const requestTimeout = 15 * time.Second

type API struct {
	Repo    *repository.Repository
	Service *service.Service
	Cfg     *config.Config
	Log     *slog.Logger

	Uploads *storage.Postgres // не nil — фото берутся из БД, а не с диска

	// Webhook бота: заполняются, только если BOT_MODE=webhook.
	WebhookPath    string
	WebhookHandler http.HandlerFunc

	limiters []*rateLimiter
}

// Close освобождает фоновые ресурсы (сборщики мусора лимитеров).
func (a *API) Close() {
	for _, rl := range a.limiters {
		rl.Close()
	}
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	// Лимиты подобраны так, чтобы живой человек их не заметил, а перебор
	// промокодов и спам заказами упирались в стену.
	browse := newRateLimiter(240, 60) // просмотр каталога
	write := newRateLimiter(20, 6)    // оформление заказа
	probe := newRateLimiter(30, 10)   // проверка промокода
	a.limiters = []*rateLimiter{browse, write, probe}

	mux.HandleFunc("GET /api/products", limit(browse, a.listProducts))
	mux.HandleFunc("GET /api/products/{id}", limit(browse, a.getProduct))
	mux.HandleFunc("GET /api/fresh-today", limit(browse, a.getFreshToday))
	mux.HandleFunc("GET /api/config", limit(browse, a.getConfig))
	mux.HandleFunc("GET /api/me", limit(browse, a.getMe))
	mux.HandleFunc("GET /api/my/orders", limit(browse, a.listMyOrders))
	mux.HandleFunc("GET /api/orders/{id}", limit(browse, a.getOrder))
	mux.HandleFunc("POST /api/orders", limit(write, a.createOrder))
	mux.HandleFunc("GET /api/promo/{code}", limit(probe, a.getPromo))
	mux.HandleFunc("GET /api/health", a.health)

	if a.Uploads != nil {
		mux.HandleFunc("GET /uploads/{file}", a.serveUpload)
	} else {
		mux.Handle("GET /uploads/", http.StripPrefix("/uploads/",
			http.FileServer(http.Dir(a.Cfg.UploadDir))))
	}

	// Приём апдейтов от Telegram, если бот работает в режиме webhook.
	if a.WebhookPath != "" && a.WebhookHandler != nil {
		mux.HandleFunc("POST "+a.WebhookPath, a.WebhookHandler)
	}

	// SPA: отдаём статику, для остальных путей — index.html.
	mux.HandleFunc("/", a.serveSPA)

	return withObservability(a.Log, requestTimeout, mux)
}

// ─── Аутентификация клиента ────────────────────────────────────────────────

// authUser проверяет initData. Если бот не настроен (локальная разработка),
// подпись проверить нечем — работаем без идентификации.
func (a *API) authUser(r *http.Request) (*TelegramUser, error) {
	return ParseInitData(
		r.Header.Get("X-Telegram-Init-Data"),
		a.Cfg.BotToken,
		a.Cfg.InitDataTTL,
		time.Now(),
	)
}

// requireUser — эндпоинты, работающие с персональными данными.
// Ошибку отдаёт сам, вызывающему остаётся выйти.
func (a *API) requireUser(w http.ResponseWriter, r *http.Request) (*TelegramUser, bool) {
	user, err := a.authUser(r)
	if err == nil {
		return user, true
	}
	switch {
	case errors.Is(err, ErrStaleInitData):
		writeError(w, http.StatusUnauthorized, "сессия устарела — закройте и откройте магазин заново")
	default:
		writeError(w, http.StatusUnauthorized, "откройте магазин через Telegram, чтобы продолжить")
	}
	return nil, false
}

// optionalUser — идентификация, если она есть; иначе nil без ошибки.
func (a *API) optionalUser(r *http.Request) *TelegramUser {
	user, err := a.authUser(r)
	if err != nil {
		return nil
	}
	return user
}

// ─── Каталог ───────────────────────────────────────────────────────────────

// productCard — карточка каталога: минимальная цена, первое фото, бейджи.
type productCard struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
	OldPrice int    `json:"old_price,omitempty"`
	Image    string `json:"image"`
	IsHit    bool   `json:"is_hit"`
	// Stock: null = учёт не ведётся, 0 = закончилось, N = осталось N.
	Stock *int `json:"stock"`
	// Признаки бейджей. Сроки и даты считает сервер по времени магазина —
	// клиент получает готовый ответ «показывать или нет».
	IsFresh     bool `json:"is_fresh,omitempty"`
	IsDailyPick bool `json:"is_daily_pick,omitempty"`
}

func (a *API) listProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	// Границы на вход: параметры уходят в поиск и сравнения, длинная строка
	// здесь бессмысленна и только жжёт CPU на расстоянии Левенштейна.
	category := clip(q.Get("category"), 32)
	filter := clip(q.Get("filter"), 32)
	search := clip(q.Get("q"), 64)

	products, err := a.Repo.ListProducts(r.Context(), category, filter, search)
	if err != nil {
		a.fail(r, w, "каталог", err, "не удалось загрузить каталог")
		return
	}

	now, today := a.Cfg.Now(), a.Cfg.Today()
	cards := make([]productCard, 0, len(products))
	for _, p := range products {
		card := productCard{
			ID: p.ID, Name: p.Name, Category: p.Category, IsHit: p.IsHit, Stock: p.Stock,
			IsFresh: p.Fresh(now), IsDailyPick: p.DailyPick(today),
		}
		if len(p.Variants) > 0 {
			card.Price = p.Variants[0].Price // варианты отсортированы по цене
			card.OldPrice = p.Variants[0].OldPrice
		}
		if len(p.Images) > 0 {
			card.Image = p.Images[0].URL
		}
		cards = append(cards, card)
	}
	writeJSON(w, http.StatusOK, cards)
}

func (a *API) getProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный id")
		return
	}
	p, err := a.Repo.GetProduct(r.Context(), uint(id))
	if err != nil || !p.Available() {
		writeError(w, http.StatusNotFound, "букет не найден")
		return
	}
	p.StampBadges(a.Cfg.Now(), a.Cfg.Today())
	writeJSON(w, http.StatusOK, p)
}

// ─── Заказы ────────────────────────────────────────────────────────────────

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}

	var in service.OrderInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	in.TelegramID = user.ID
	in.IdempotencyKey = clip(r.Header.Get("Idempotency-Key"), 64)
	if in.Name == "" {
		in.Name = user.FullName() // подстраховка: имя из профиля Telegram
	}

	order, err := a.Service.CreateOrder(r.Context(), in)
	if err != nil {
		var ve *service.ValidationError
		if errors.As(err, &ve) {
			body := map[string]any{"error": ve.Msg}
			if len(ve.UnavailableVariants) > 0 {
				body["unavailable_variant_ids"] = ve.UnavailableVariants
			}
			writeJSON(w, http.StatusBadRequest, body)
			return
		}
		a.fail(r, w, "создание заказа", err, "не удалось создать заказ, попробуйте ещё раз")
		return
	}
	writeJSON(w, http.StatusCreated, a.orderView(order))
}

func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный id")
		return
	}
	order, err := a.Repo.GetOrder(r.Context(), uint(id))
	// Чужой заказ неотличим от несуществующего — иначе перебором id
	// можно узнать, какие номера заказов существуют.
	if err != nil || order.User.TelegramID != user.ID {
		writeError(w, http.StatusNotFound, "заказ не найден")
		return
	}
	writeJSON(w, http.StatusOK, a.orderView(order))
}

// listMyOrders — «мои заказы»: клиент видит статус, не спрашивая менеджера.
func (a *API) listMyOrders(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireUser(w, r)
	if !ok {
		return
	}
	dbUser, err := a.Repo.GetUserByTelegramID(r.Context(), user.ID)
	if err != nil {
		writeJSON(w, http.StatusOK, []orderView{})
		return
	}
	orders, err := a.Repo.ListUserOrders(r.Context(), dbUser.ID, 20)
	if err != nil {
		a.fail(r, w, "список заказов", err, "не удалось загрузить заказы")
		return
	}
	out := make([]orderView, 0, len(orders))
	for i := range orders {
		out = append(out, a.orderView(&orders[i]))
	}
	writeJSON(w, http.StatusOK, out)
}

// orderView — то, что видит клиент. Отдаём только его собственные данные
// и ничего служебного (ключ идемпотентности, внутренние id пользователя).
type orderView struct {
	ID              uint            `json:"id"`
	Status          string          `json:"status"`
	StatusLabel     string          `json:"status_label"`
	SubtotalPrice   int             `json:"subtotal_price"`
	DiscountAmount  int             `json:"discount_amount"`
	TotalPrice      int             `json:"total_price"`
	DeliveryAddress string          `json:"delivery_address"`
	DeliveryDate    string          `json:"delivery_date"`
	DeliveryTime    string          `json:"delivery_time"`
	Comment         string          `json:"comment"`
	CardText        string          `json:"card_text"`
	IsAnonymous     bool            `json:"is_anonymous"`
	CancelReason    string          `json:"cancel_reason,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	Items           []orderItemView `json:"items"`
	PromoCode       *promoView      `json:"promo_code,omitempty"`
}

type orderItemView struct {
	ID          uint   `json:"id"`
	ProductName string `json:"product_name"`
	// ProductID и VariantID нужны экрану «повторить заказ»: по ним корзина
	// пересобирается по актуальным ценам, а не по ценам полугодовой давности.
	ProductID    uint `json:"product_id"`
	VariantID    uint `json:"variant_id"`
	FlowersCount int  `json:"flowers_count"`
	Quantity     int  `json:"quantity"`
	Price        int  `json:"price"`
}

// promoView — представление промокода для Mini App.
//
// discount_percent остаётся первым полем и означает ровно то же, что и раньше:
// процент у процентных кодов и 0 у фиксированных. Остальные поля добавлены
// сверху, поэтому старый фронтенд продолжает работать без изменений.
type promoView struct {
	Code            string `json:"code"`
	DiscountPercent int    `json:"discount_percent"`
	DiscountType    string `json:"discount_type,omitempty"`
	DiscountValue   int    `json:"discount_value,omitempty"`
	MinOrderAmount  int    `json:"min_order_amount,omitempty"`
	// Заполняются, только если известна сумма корзины (?subtotal=).
	DiscountAmount int `json:"discount_amount,omitempty"`
	Total          int `json:"total,omitempty"`
}

func newPromoView(p *model.PromoCode) promoView {
	return promoView{
		Code:            p.Code,
		DiscountPercent: p.Percent(),
		DiscountType:    p.DiscountType,
		DiscountValue:   p.DiscountValue,
		MinOrderAmount:  p.MinOrderAmount,
	}
}

func (a *API) orderView(o *model.Order) orderView {
	v := orderView{
		ID:              o.ID,
		Status:          o.Status,
		StatusLabel:     model.StatusLabels[o.Status],
		SubtotalPrice:   o.SubtotalPrice,
		DiscountAmount:  o.DiscountAmount,
		TotalPrice:      o.TotalPrice,
		DeliveryAddress: o.DeliveryAddress,
		DeliveryDate:    o.DeliveryDate,
		DeliveryTime:    o.DeliveryTime,
		Comment:         o.Comment,
		CardText:        o.CardText,
		IsAnonymous:     o.IsAnonymous,
		CancelReason:    o.CancelReason,
		CreatedAt:       o.CreatedAt,
		Items:           make([]orderItemView, 0, len(o.Items)),
	}
	for _, it := range o.Items {
		v.Items = append(v.Items, orderItemView{
			ID:           it.ID,
			ProductName:  it.ProductName,
			ProductID:    it.Variant.ProductID,
			VariantID:    it.VariantID,
			FlowersCount: it.Variant.Quantity,
			Quantity:     it.Quantity,
			Price:        it.Price,
		})
	}
	switch {
	case o.PromoCode != nil:
		pv := newPromoView(o.PromoCode)
		pv.DiscountAmount = o.DiscountAmount
		v.PromoCode = &pv
	case o.AppliedPromoCode != "":
		// Промокод удалили после заказа: показываем снимок кода из самого заказа,
		// иначе у клиента скидка «ниоткуда».
		v.PromoCode = &promoView{Code: o.AppliedPromoCode, DiscountAmount: o.DiscountAmount}
	}
	return v
}

// ─── Промокоды и профиль ───────────────────────────────────────────────────

func (a *API) getPromo(w http.ResponseWriter, r *http.Request) {
	var tgID int64
	if u := a.optionalUser(r); u != nil {
		tgID = u.ID
	}
	// subtotal — необязательная сумма корзины. Если она пришла, отвечаем сразу
	// и суммой скидки: считает её всё равно сервер, клиент только показывает.
	subtotal, _ := strconv.Atoi(r.URL.Query().Get("subtotal"))
	if subtotal < 0 {
		subtotal = 0
	}

	promo, err := a.Service.CheckPromo(r.Context(), tgID, clip(r.PathValue("code"), 64), subtotal)
	if err != nil {
		var ve *service.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusNotFound, ve.Msg)
			return
		}
		a.fail(r, w, "проверка промокода", err, "не удалось проверить промокод")
		return
	}
	view := newPromoView(promo)
	if subtotal > 0 {
		view.DiscountAmount, view.Total = promo.Apply(subtotal)
	}
	writeJSON(w, http.StatusOK, view)
}

// getMe — сохранённые контакты и промокод из deep-link.
func (a *API) getMe(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{}
	user := a.optionalUser(r)
	if user == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp["telegram_name"] = user.FullName()
	if dbUser, err := a.Repo.GetUserByTelegramID(r.Context(), user.ID); err == nil {
		resp["name"] = dbUser.Name
		resp["phone"] = dbUser.Phone
		if dbUser.PromoCode != nil && dbUser.PromoCode.Usable(a.Cfg.Now()) {
			resp["promo_code"] = dbUser.PromoCode.Code
			resp["discount_percent"] = dbUser.PromoCode.Percent()
			resp["discount_type"] = dbUser.PromoCode.DiscountType
			resp["discount_value"] = dbUser.PromoCode.DiscountValue
			resp["min_order_amount"] = dbUser.PromoCode.MinOrderAmount
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// getFreshToday — блок «Сегодня на базе» для главной.
func (a *API) getFreshToday(w http.ResponseWriter, r *http.Request) {
	fresh, err := a.Repo.GetFreshToday(r.Context(), a.Cfg.Today())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": fresh.Items})
}

// getConfig отдаёт фронтенду правила доставки, чтобы форма не предлагала
// вариантов, которые сервер всё равно отклонит.
func (a *API) getConfig(w http.ResponseWriter, r *http.Request) {
	now := a.Cfg.Now()
	nowMin := now.Hour()*60 + now.Minute()
	open, closeAt := a.Cfg.ShopOpenHour*60, a.Cfg.ShopCloseHour*60

	// Экспресс возможен, только если магазин открыт прямо сейчас.
	expressAvailable := nowMin >= open && nowMin <= closeAt
	// Точное время на сегодня — не раньше чем через час и до закрытия.
	// Округляем вверх до четверти часа: слот «к 19:42» не выбирают,
	// а поле времени в браузере шагает по 15 минут.
	earliestToday := ""
	if t := roundUpTo(max(nowMin+60, open), 15); t <= closeAt {
		earliestToday = fmtMinutes(t)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"today":             a.Cfg.Today(),
		"open_hour":         a.Cfg.ShopOpenHour,
		"close_hour":        a.Cfg.ShopCloseHour,
		"express_available": expressAvailable,
		"earliest_today":    earliestToday, // "" = сегодня ко времени уже не успеть
		"max_preorder_days": 60,
	})
}

func fmtMinutes(m int) string {
	return pad2(m/60) + ":" + pad2(m%60)
}

// roundUpTo округляет минуты вверх до кратности step.
func roundUpTo(m, step int) int {
	if r := m % step; r != 0 {
		m += step - r
	}
	return m
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// ─── Служебное ─────────────────────────────────────────────────────────────

// health отражает реальное состояние зависимостей: без живой БД сервис
// бесполезен, и Railway должен об этом знать.
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := a.Repo.Ping(ctx); err != nil {
		LoggerFrom(r.Context(), a.Log).Error("health: БД недоступна", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "degraded",
			"db":     "down",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "db": "up"})
}

// serveUpload — отдаёт фото товара из БД (хостинг без постоянного диска).
func (a *API) serveUpload(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("file")
	up, err := a.Uploads.Get(r.Context(), name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", up.MimeType)
	// id файла неизменяем — можно кэшировать навсегда.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, up.CreatedAt, bytes.NewReader(up.Data))
}

func (a *API) serveSPA(w http.ResponseWriter, r *http.Request) {
	// Служебные префиксы не должны проваливаться в SPA: иначе перебор
	// секретного пути webhook отвечает 200 и index.html вместо 404.
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/telegram/") {
		writeError(w, http.StatusNotFound, "не найдено")
		return
	}

	clean := path.Clean("/" + r.URL.Path)
	file := filepath.Join(a.Cfg.WebDist, filepath.FromSlash(clean))
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		// Ассеты Vite содержат хеш в имени — безопасно кэшировать навсегда.
		if strings.HasPrefix(clean, "/assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeFile(w, r, file)
		return
	}
	// index.html кэшировать нельзя: после редеплоя он ссылается на новые
	// хеши ассетов, а старый закэшированный даёт белый экран.
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	http.ServeFile(w, r, filepath.Join(a.Cfg.WebDist, "index.html"))
}

// fail логирует настоящую ошибку с контекстом и отдаёт клиенту безопасный текст.
func (a *API) fail(r *http.Request, w http.ResponseWriter, op string, err error, publicMsg string) {
	LoggerFrom(r.Context(), a.Log).Error("ошибка обработки", "op", op, "err", err)
	writeError(w, http.StatusInternalServerError, publicMsg)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	buf, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"внутренняя ошибка"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// clip обрезает строку по числу рун — граница на любой вход извне.
func clip(s string, max int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return string(r)
}
