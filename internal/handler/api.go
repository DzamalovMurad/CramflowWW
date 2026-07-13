package handler

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
)

type API struct {
	Repo      *repository.Repository
	Service   *service.Service
	BotToken  string
	UploadDir string // локальные фото, отдаются по /uploads/
	WebDist   string // собранный фронтенд
}

func (a *API) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/products", a.listProducts)
	mux.HandleFunc("GET /api/products/{id}", a.getProduct)
	mux.HandleFunc("POST /api/orders", a.createOrder)
	mux.HandleFunc("GET /api/orders/{id}", a.getOrder)
	mux.HandleFunc("GET /api/promo/{code}", a.getPromo)
	mux.HandleFunc("GET /api/me", a.getMe)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.Handle("GET /uploads/", http.StripPrefix("/uploads/",
		http.FileServer(http.Dir(a.UploadDir))))

	// SPA: отдаём статику, для остальных путей — index.html.
	mux.HandleFunc("/", a.serveSPA)

	return mux
}

// productCard — карточка каталога: минимальная цена и первое фото.
type productCard struct {
	ID       uint   `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Price    int    `json:"price"`
	Image    string `json:"image"`
}

func (a *API) listProducts(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	filter := r.URL.Query().Get("filter")

	products, err := a.Repo.ListProducts(category, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось загрузить каталог")
		return
	}

	cards := make([]productCard, 0, len(products))
	for _, p := range products {
		card := productCard{ID: p.ID, Name: p.Name, Category: p.Category}
		if len(p.Variants) > 0 {
			card.Price = p.Variants[0].Price // варианты отсортированы по цене
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
	in.TelegramID = telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken)

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

func (a *API) serveSPA(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
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
