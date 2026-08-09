package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ctxKey int

const (
	ctxKeyRequestID ctxKey = iota
	ctxKeyLogger
)

// RequestID достаёт корреляционный идентификатор запроса.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyRequestID).(string)
	return id
}

// LoggerFrom — логгер запроса (уже с request_id).
func LoggerFrom(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(ctxKeyLogger).(*slog.Logger); ok {
		return l
	}
	return fallback
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// statusWriter запоминает код ответа для лога.
type statusWriter struct {
	http.ResponseWriter
	status int
	wrote  int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.wrote += n
	return n, err
}

// withObservability вешает на запрос идентификатор, логгер и таймаут,
// ловит панику и пишет одну структурированную строку на запрос.
func withObservability(log *slog.Logger, timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := newID()
		reqLog := log.With("request_id", id, "method", r.Method, "path", r.URL.Path)

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		ctx = context.WithValue(ctx, ctxKeyRequestID, id)
		ctx = context.WithValue(ctx, ctxKeyLogger, reqLog)

		sw := &statusWriter{ResponseWriter: w}
		sw.Header().Set("X-Request-Id", id)

		defer func() {
			if rec := recover(); rec != nil {
				reqLog.Error("паника в обработчике", "panic", rec)
				if sw.status == 0 {
					writeError(sw, http.StatusInternalServerError, "внутренняя ошибка")
				}
			}
			level := slog.LevelInfo
			if sw.status >= 500 {
				level = slog.LevelError
			}
			reqLog.Log(ctx, level, "http",
				"status", sw.status,
				"bytes", sw.wrote,
				"duration_ms", time.Since(start).Milliseconds())
		}()

		next.ServeHTTP(sw, r.WithContext(ctx))
	})
}

// ─── Rate limiting ─────────────────────────────────────────────────────────

// rateLimiter — token bucket на ключ (IP или telegram_id), в памяти.
// Для одного инстанса на Railway этого достаточно; распределённого стора
// в проекте нет и заводить его ради одного сервиса незачем.
type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64 // токенов в секунду
	burst   float64
	stop    chan struct{}
}

type bucket struct {
	tokens float64
	seen   time.Time
}

func newRateLimiter(perMinute, burst int) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*bucket),
		rate:    float64(perMinute) / 60,
		burst:   float64(burst),
		stop:    make(chan struct{}),
	}
	go rl.janitor()
	return rl
}

// janitor выкидывает давно неактивные ведра, чтобы map не рос бесконечно.
func (rl *rateLimiter) janitor() {
	t := time.NewTicker(5 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			cutoff := time.Now().Add(-10 * time.Minute)
			rl.mu.Lock()
			for k, b := range rl.buckets {
				if b.seen.Before(cutoff) {
					delete(rl.buckets, k)
				}
			}
			rl.mu.Unlock()
		case <-rl.stop:
			return
		}
	}
}

func (rl *rateLimiter) Close() { close(rl.stop) }

// Allow списывает токен. false = превышен лимит.
func (rl *rateLimiter) Allow(key string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		rl.buckets[key] = &bucket{tokens: rl.burst - 1, seen: now}
		return true
	}
	b.tokens = min(rl.burst, b.tokens+now.Sub(b.seen).Seconds()*rl.rate)
	b.seen = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// limit — middleware поверх конкретного обработчика.
func limit(rl *rateLimiter, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !rl.Allow(clientIP(r)) {
			w.Header().Set("Retry-After", "30")
			writeError(w, http.StatusTooManyRequests, "слишком много запросов — подождите немного")
			return
		}
		next(w, r)
	}
}

// clientIP — адрес клиента с учётом прокси Railway (X-Forwarded-For).
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Первый адрес в списке — исходный клиент.
		if i := strings.IndexByte(xff, ','); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
