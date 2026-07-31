package handler

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/observability"
	"github.com/dzamalovmurad/cramflowww/internal/ratelimit"
)

// --- Request ID + логирование ---

// HeaderRequestID — заголовок, по которому запрос ищется в логах.
// Если балансировщик уже проставил свой id, уважаем его.
const HeaderRequestID = "X-Request-Id"

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// statusWriter запоминает код ответа и объём тела для лога.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Flush нужен, чтобы обёртка не ломала http.ServeContent и SPA-раздачу.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// withObservability — request id, лог доступа и перехват паник.
// Паника в хендлере не роняет процесс: клиент получает 500, стек — в Sentry.
func withObservability(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(HeaderRequestID, id)

		r = r.WithContext(observability.WithRequestID(r.Context(), id))
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()

		defer func() {
			if rec := recover(); rec != nil {
				observability.CapturePanic(rec, map[string]string{
					"component":  "http",
					"request_id": id,
					"path":       r.URL.Path,
				})
				if sw.status == 0 {
					writeError(sw, http.StatusInternalServerError, "внутренняя ошибка")
				}
			}
			// Статику и фото не логируем — иначе лог заливает раздача картинок.
			if isNoisyPath(r.URL.Path) {
				return
			}
			log.Printf("%s %s %s → %d (%s, %d B) rid=%s",
				clientIP(r), r.Method, r.URL.Path, sw.statusOr200(),
				time.Since(start).Round(time.Millisecond), sw.bytes, id)
		}()

		next.ServeHTTP(sw, r)
	})
}

func (w *statusWriter) statusOr200() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func isNoisyPath(path string) bool {
	return strings.HasPrefix(path, "/uploads/") ||
		strings.HasPrefix(path, "/assets/") ||
		strings.HasPrefix(path, "/seed/") ||
		strings.HasPrefix(path, "/fonts/")
}

// clientIP — адрес клиента с учётом прокси Railway/Render.
func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i > 0 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// --- Rate limiting ---

// Лимиты подобраны под живого человека в Mini App: пролистать каталог,
// открыть десяток карточек, оформить заказ. Скрипт в цикле упирается сразу.
const (
	readRPS    = 5  // GET-запросы: 5 в секунду в среднем…
	readBurst  = 30 // …со всплеском на 30 (открытие приложения греет каталог)
	writeRPS   = 0.2
	writeBurst = 5 // не больше 5 заказов подряд, дальше — раз в 5 секунд
)

// rateLimiters — отдельные корзины на чтение и на запись: тяжёлый POST
// не должен блокироваться из-за пролистывания каталога, и наоборот.
type rateLimiters struct {
	read  *ratelimit.Limiter
	write *ratelimit.Limiter
}

func newRateLimiters() *rateLimiters {
	return &rateLimiters{
		read:  ratelimit.New(readRPS, readBurst),
		write: ratelimit.New(writeRPS, writeBurst),
	}
}

// limitKey — ключ корзины: telegram id пользователя, если initData валидна,
// иначе IP. Подделать telegram id нельзя — подпись проверяется ботом.
func (a *API) limitKey(r *http.Request) string {
	if id := telegramUserID(r.Header.Get("X-Telegram-Init-Data"), a.BotToken); id != 0 {
		return "tg:" + strconv.FormatInt(id, 10)
	}
	return "ip:" + clientIP(r)
}

// withRateLimit оборачивает клиентский API. Ответ 429 с Retry-After —
// фронтенд показывает обычную ошибку, ретраить сам не пытается.
func (a *API) withRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lim := a.limiters.read
		retry := 1
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			lim = a.limiters.write
			retry = 5
		}
		if !lim.Allow(a.limitKey(r)) {
			w.Header().Set("Retry-After", strconv.Itoa(retry))
			writeError(w, http.StatusTooManyRequests, "слишком часто — подождите пару секунд")
			return
		}
		next.ServeHTTP(w, r)
	})
}
