package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/observability"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

type API struct {
	Repo      *repository.Repository
	Service   *service.Service
	BotToken  string
	UploadDir string            // локальные фото, отдаются по /uploads/
	Uploads   *storage.Postgres // если задан — фото берутся из БД, а не с диска
	WebDist   string            // собранный фронтенд

	// Health — зависимости для /healthz (БД и Bot API).
	Health Healthchecker
	// BotUsername — для кнопки «открыть чат с ботом» в экране ошибки Mini App.
	BotUsername string
	// AppURL — публичный адрес Mini App. Пустой = витрина работает только
	// через бота, фронтенд об этом узнаёт из /api/config.
	AppURL string

	// Webhook бота: заполняются, только если BOT_MODE=webhook.
	WebhookPath    string
	WebhookHandler http.HandlerFunc

	limiters *rateLimiters
	botPing  botPingCache
}

func (a *API) Routes() http.Handler {
	if a.limiters == nil {
		a.limiters = newRateLimiters()
	}

	// Клиентский API — под лимитером: один пользователь не должен выжимать
	// пул соединений к Postgres.
	api := http.NewServeMux()
	api.HandleFunc("GET /api/products", a.listProducts)
	api.HandleFunc("GET /api/products/hits", a.listHits)
	api.HandleFunc("GET /api/products/{id}", a.getProduct)
	api.HandleFunc("POST /api/orders", a.createOrder)
	api.HandleFunc("GET /api/orders/{id}", a.getOrder)
	api.HandleFunc("POST /api/cart/check", a.checkCart)
	api.HandleFunc("POST /api/cart/touch", a.touchCart)
	api.HandleFunc("GET /api/promo/{code}", a.getPromo)
	api.HandleFunc("GET /api/me", a.getMe)
	api.HandleFunc("GET /api/fresh-today", a.getFreshToday)
	api.HandleFunc("GET /api/config", a.getConfig)
	// Лёгкая проба живости: БД не трогает — по ней keepalive будит контейнер,
	// не мешая serverless-Postgres спать. Глубокая проверка — в /healthz.
	api.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux := http.NewServeMux()
	mux.Handle("/api/", a.withRateLimit(api))

	// Healthcheck Railway: без лимитера — иначе проба сама себя заблокирует.
	if a.Health != nil {
		mux.HandleFunc("GET /healthz", a.healthz)
	}

	if a.Uploads != nil {
		mux.HandleFunc("GET /uploads/{file}", a.serveUpload)
	} else {
		mux.Handle("GET /uploads/", http.StripPrefix("/uploads/",
			http.FileServer(http.Dir(a.UploadDir))))
	}

	// Приём апдейтов от Telegram, если бот работает в режиме webhook.
	if a.WebhookPath != "" && a.WebhookHandler != nil {
		mux.HandleFunc("POST "+a.WebhookPath, a.WebhookHandler)
	}

	// SPA: отдаём статику, для остальных путей — index.html.
	mux.HandleFunc("/", a.serveSPA)

	return withObservability(mux)
}

// productCard — карточка каталога: минимальная цена, первое фото, бейджи.
type productCard struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
	OldPrice int    `json:"old_price,omitempty"` // старая цена минимального варианта
	Image    string `json:"image"`
	IsHit    bool   `json:"is_hit"`
	LowStock bool   `json:"low_stock,omitempty"` // ручной бейдж «мало осталось»
	Seasonal bool   `json:"seasonal,omitempty"`  // товар есть в «сегодня на базе»
	Stock    int    `json:"stock,omitempty"`     // остаток для бейджа «осталось N»
}

func (a *API) card(p *model.Product, seasonal []string) productCard {
	card := productCard{
		ID:       p.ID,
		Name:     p.Name,
		Category: p.Category,
		IsHit:    p.IsHit,
		LowStock: p.LowStock,
		Stock:    p.Stock,
		Seasonal: repository.IsSeasonal(p.Name, seasonal),
	}
	if len(p.Variants) > 0 {
		card.Price = p.Variants[0].Price // варианты отсортированы по цене
		card.OldPrice = p.Variants[0].OldPrice
	}
	if len(p.Images) > 0 {
		card.Image = p.Images[0].URL
	}
	return card
}

func (a *API) listProducts(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	filter := r.URL.Query().Get("filter")
	search := r.URL.Query().Get("q")

	products, err := a.Repo.ListProducts(category, filter, search)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось загрузить каталог")
		return
	}

	seasonal := a.Repo.SeasonalNames(time.Now().Format("2006-01-02"))
	cards := make([]productCard, 0, len(products))
	for i := range products {
		cards = append(cards, a.card(&products[i], seasonal))
	}
	writeJSON(w, http.StatusOK, cards)
}

// listHits — подборка хитов (пустая корзина в Mini App). Порядок — sort_order.
func (a *API) listHits(w http.ResponseWriter, r *http.Request) {
	limit := 5
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 20 {
		limit = v
	}
	products, err := a.Repo.TopHits(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось загрузить подборку")
		return
	}
	seasonal := a.Repo.SeasonalNames(time.Now().Format("2006-01-02"))
	cards := make([]productCard, 0, len(products))
	for i := range products {
		cards = append(cards, a.card(&products[i], seasonal))
	}
	writeJSON(w, http.StatusOK, cards)
}

// getConfig — то, что фронтенду нужно знать о развёрнутом сервисе:
// куда вести пользователя, если Mini App упал, и включён ли заказ через бота.
func (a *API) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"bot_username":    a.BotUsername,
		"fallback_orders": fallbackOrdersOn(a.Repo, a.AppURL),
		"mini_app_url":    a.AppURL,
	})
}

func (a *API) getProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный id")
		return
	}
	p, err := a.Repo.GetProduct(uint(id))
	if err != nil || p.IsHidden || !p.IsAvailable {
		writeError(w, http.StatusNotFound, "товар не найден")
		return
	}
	// Сезонность вычисляется из «сегодня на базе», в самом товаре её нет.
	seasonal := a.Repo.SeasonalNames(time.Now().Format("2006-01-02"))
	writeJSON(w, http.StatusOK, struct {
		*model.Product
		Seasonal bool `json:"seasonal"`
	}{p, repository.IsSeasonal(p.Name, seasonal)})
}

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var in service.OrderInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	in.TelegramID = telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)

	order, err := a.Service.CreateOrder(in)
	if err != nil {
		var ve *service.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Msg)
			return
		}
		observability.CaptureError(err, map[string]string{
			"component":  "api",
			"op":         "create_order",
			"request_id": observability.RequestID(r.Context()),
		})
		writeError(w, http.StatusInternalServerError, "не удалось создать заказ")
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный id")
		return
	}
	order, err := a.Repo.GetOrder(uint(id))
	if err != nil {
		writeError(w, http.StatusNotFound, "заказ не найден")
		return
	}
	// Чужие заказы не показываем: доступ только владельцу по initData.
	tgID := telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)
	if order.User.TelegramID != tgID {
		writeError(w, http.StatusNotFound, "заказ не найден")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

// checkCart — какие позиции корзины больше нельзя заказать. Корзина живёт в
// localStorage браузера и легко переживает снятие товара с наличия, поэтому
// Mini App сверяется с бэкендом при открытии корзины и перед оформлением.
func (a *API) checkCart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		VariantIDs []uint `json:"variant_ids"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	// Ограничение на размер корзины: защита от запроса с тысячей id.
	if len(in.VariantIDs) > 100 {
		in.VariantIDs = in.VariantIDs[:100]
	}

	unavailable, err := a.Repo.UnavailableVariants(in.VariantIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось проверить корзину")
		return
	}
	if unavailable == nil {
		unavailable = []uint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"unavailable": unavailable})
}

// touchCart — отметка активности в корзине для сегмента рассылки
// «корзина без заказа». Пишем только для известного пользователя Telegram.
func (a *API) touchCart(w http.ResponseWriter, r *http.Request) {
	tgID := telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)
	if tgID != 0 {
		if err := a.Repo.TouchCart(tgID); err != nil {
			log.Printf("touch cart: %v", err)
		}
	}
	// Ответ всегда 200: это фоновая телеметрия, она не должна ломать корзину.
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getPromo — проверка промокода из формы checkout (показать скидку до оформления).
func (a *API) getPromo(w http.ResponseWriter, r *http.Request) {
	promo, err := a.Repo.GetPromoByCode(r.PathValue("code"))
	if err != nil {
		writeError(w, http.StatusNotFound, "промокод не найден")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code":             promo.Code,
		"discount_percent": promo.DiscountPercent,
	})
}

// getMe — сохранённый по deep-link промокод текущего пользователя.
func (a *API) getMe(w http.ResponseWriter, r *http.Request) {
	tgID := telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)
	resp := map[string]any{}
	if tgID != 0 {
		if user, err := a.Repo.GetUserByTelegramID(tgID); err == nil {
			resp["name"] = user.Name
			resp["phone"] = user.Phone
			if user.PromoCode != nil {
				resp["promo_code"] = user.PromoCode.Code
				resp["discount_percent"] = user.PromoCode.DiscountPercent
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// getFreshToday — блок «Сегодня на базе» для главной. Пустой объект, если записи за сегодня нет.
func (a *API) getFreshToday(w http.ResponseWriter, _ *http.Request) {
	fresh, err := a.Repo.GetFreshToday(time.Now().Format("2006-01-02"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": fresh.Items})
}

// serveUpload — отдаёт фото товара из БД (хостинг без постоянного диска).
func (a *API) serveUpload(w http.ResponseWriter, r *http.Request) {
	up, err := a.Uploads.Get(r.PathValue("file"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", up.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // id неизменяем
	w.Header().Set("Content-Length", strconv.Itoa(len(up.Data)))
	http.ServeContent(w, r, r.PathValue("file"), up.CreatedAt, bytes.NewReader(up.Data))
}

func (a *API) serveSPA(w http.ResponseWriter, r *http.Request) {
	// Служебные префиксы не должны проваливаться в SPA: иначе перебор
	// секретного пути webhook отвечает 200 и index.html вместо 404.
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/telegram/") {
		writeError(w, http.StatusNotFound, "не найдено")
		return
	}
	path := filepath.Join(a.WebDist, filepath.Clean("/"+r.URL.Path))
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		http.ServeFile(w, r, path)
		return
	}
	http.ServeFile(w, r, filepath.Join(a.WebDist, "index.html"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
