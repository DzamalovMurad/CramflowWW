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

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

type API struct {
	Repo      *repository.Repository
	Service   *service.Service
	BotToken  string
	Bot       *Bot              // nil без BOT_TOKEN: проверка подписки на канал недоступна
	UploadDir string            // локальные фото, отдаются по /uploads/
	Uploads   *storage.Postgres // если задан — фото берутся из БД, а не с диска
	WebDist   string            // собранный фронтенд

	// Webhook бота: заполняются, только если BOT_MODE=webhook.
	WebhookPath    string
	WebhookHandler http.HandlerFunc
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/products", a.listProducts)
	mux.HandleFunc("GET /api/products/{id}", a.getProduct)
	mux.HandleFunc("POST /api/orders", a.createOrder)
	mux.HandleFunc("GET /api/orders/{id}", a.getOrder)
	mux.HandleFunc("GET /api/promo/{code}", a.getPromo)
	mux.HandleFunc("GET /api/me", a.getMe)
	mux.HandleFunc("GET /api/fresh-today", a.getFreshToday)
	mux.HandleFunc("POST /api/launch", a.trackLaunch)
	mux.HandleFunc("GET /api/subscription-bonus", a.getSubscriptionBonus)
	mux.HandleFunc("POST /api/subscription-bonus", a.claimSubscriptionBonus)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

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

	return mux
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
	Stock    int    `json:"stock,omitempty"` // остаток для бейджа «осталось N»
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

	cards := make([]productCard, 0, len(products))
	for _, p := range products {
		card := productCard{ID: p.ID, Name: p.Name, Category: p.Category, IsHit: p.IsHit, Stock: p.Stock}
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
	p, err := a.Repo.GetProduct(uint(id))
	if err != nil || p.IsHidden {
		writeError(w, http.StatusNotFound, "товар не найден")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var in service.OrderInput
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	tgID, startParam := telegramLaunch(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)
	in.TelegramID = tgID
	in.Source = model.SourceFromStartParam(startParam)

	order, err := a.Service.CreateOrder(in)
	if err != nil {
		var ve *service.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, ve.Msg)
			return
		}
		log.Printf("create order: %v", err)
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

// trackLaunch — вызывается фронтендом при каждом запуске Mini App: парсит
// start_param из initData и записывает first-touch источник клиента
// (acquisition_source заполняется один раз и больше не перезаписывается).
func (a *API) trackLaunch(w http.ResponseWriter, r *http.Request) {
	tgID, startParam := telegramLaunch(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)
	if tgID != 0 {
		if source := model.SourceFromStartParam(startParam); source != "" {
			if user, err := a.Repo.UpsertUser(tgID, "", ""); err == nil {
				_ = a.Repo.SetUserAcquisitionSource(user.ID, source)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

// subscriptionBonusState — состояние бонуса за подписку для Mini App.
func (a *API) subscriptionBonusState(tgID int64) map[string]any {
	resp := map[string]any{
		"enabled": a.Bot != nil && a.Bot.ChannelEnabled(),
		"claimed": false,
	}
	if a.Bot != nil {
		if url := a.Bot.ChannelURL(); url != "" {
			resp["channel_url"] = url
		}
	}
	if tgID == 0 {
		return resp
	}
	user, err := a.Repo.GetUserByTelegramID(tgID)
	if err != nil {
		return resp
	}
	if cb, err := a.Repo.GetClaimedBonus(user.ID, model.BonusSubscription); err == nil {
		resp["claimed"] = true
		resp["code"] = cb.PromoCode.Code
		resp["discount_percent"] = cb.PromoCode.DiscountPercent
	}
	return resp
}

// getSubscriptionBonus — статус блока «Промокод за подписку» в профиле.
func (a *API) getSubscriptionBonus(w http.ResponseWriter, r *http.Request) {
	tgID := telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)
	writeJSON(w, http.StatusOK, a.subscriptionBonusState(tgID))
}

// claimSubscriptionBonus — выдача одноразового промокода за подписку на канал.
// Мягкий гейт: getChatMember подтверждает подписку, claimed_bonuses не даёт
// получить бонус повторно (уникальный индекс user_id+type).
func (a *API) claimSubscriptionBonus(w http.ResponseWriter, r *http.Request) {
	if a.Bot == nil || !a.Bot.ChannelEnabled() {
		writeError(w, http.StatusServiceUnavailable, "бонус за подписку сейчас недоступен")
		return
	}
	tgID := telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)
	if tgID == 0 {
		writeError(w, http.StatusUnauthorized, "откройте магазин из Telegram, чтобы получить бонус")
		return
	}
	user, err := a.Repo.UpsertUser(tgID, "", "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "что-то пошло не так, попробуйте позже")
		return
	}
	// Уже получали — просто возвращаем текущее состояние с кодом.
	if _, err := a.Repo.GetClaimedBonus(user.ID, model.BonusSubscription); err == nil {
		writeJSON(w, http.StatusOK, a.subscriptionBonusState(tgID))
		return
	}

	subscribed, err := a.Bot.IsSubscribed(tgID)
	if err != nil {
		log.Printf("проверка подписки: %v", err)
		writeError(w, http.StatusBadGateway, "не получилось проверить подписку, попробуйте позже")
		return
	}
	if !subscribed {
		writeError(w, http.StatusForbidden, "сначала подпишитесь на канал — и возвращайтесь за промокодом 🌸")
		return
	}

	promo, err := a.Repo.CreateSingleUsePromo("FLOWIX", model.SubscriptionPromoPercent)
	if err != nil {
		log.Printf("промокод за подписку: %v", err)
		writeError(w, http.StatusInternalServerError, "что-то пошло не так, попробуйте позже")
		return
	}
	err = a.Repo.CreateClaimedBonus(&model.ClaimedBonus{
		UserID: user.ID, Type: model.BonusSubscription, PromoCodeID: promo.ID,
	})
	if err != nil {
		// Гонка двух запросов: бонус уже выдан — отдаём сохранённый.
		if !errors.Is(err, gorm.ErrDuplicatedKey) {
			log.Printf("фиксация бонуса за подписку: %v", err)
			writeError(w, http.StatusInternalServerError, "что-то пошло не так, попробуйте позже")
			return
		}
	}
	writeJSON(w, http.StatusOK, a.subscriptionBonusState(tgID))
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
