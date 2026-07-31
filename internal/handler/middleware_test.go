package handler

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRateLimiterBlocksBurst(t *testing.T) {
	rl := newRateLimiter(60, 3) // 1 запрос в секунду, всплеск 3
	defer rl.Close()

	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("запрос %d из всплеска должен проходить", i+1)
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("четвёртый запрос подряд должен быть отклонён")
	}
	// Другой клиент не страдает от соседа.
	if !rl.Allow("5.6.7.8") {
		t.Fatal("лимит одного клиента не должен влиять на другого")
	}
}

func TestRateLimiterRefills(t *testing.T) {
	rl := newRateLimiter(6000, 1) // 100 запросов в секунду
	defer rl.Close()

	if !rl.Allow("k") {
		t.Fatal("первый запрос должен проходить")
	}
	if rl.Allow("k") {
		t.Fatal("второй сразу — нет")
	}
	time.Sleep(50 * time.Millisecond) // хватит на несколько токенов
	if !rl.Allow("k") {
		t.Fatal("после паузы токен должен восстановиться")
	}
}

func TestLimitMiddlewareReturns429(t *testing.T) {
	rl := newRateLimiter(60, 1)
	defer rl.Close()

	h := limit(rl, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/products", nil)
	req.RemoteAddr = "9.9.9.9:1234"

	first := httptest.NewRecorder()
	h(first, req)
	if first.Code != http.StatusOK {
		t.Fatalf("первый запрос: %d", first.Code)
	}

	second := httptest.NewRecorder()
	h(second, req)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("второй запрос: %d, ожидали 429", second.Code)
	}
	if second.Header().Get("Retry-After") == "" {
		t.Error("нет заголовка Retry-After")
	}
}

func TestClientIPPrefersForwardedFor(t *testing.T) {
	cases := map[string]struct {
		xff, remote, want string
	}{
		"без прокси":     {"", "203.0.113.9:5555", "203.0.113.9"},
		"один адрес":     {"198.51.100.7", "10.0.0.1:1", "198.51.100.7"},
		"цепочка прокси": {"198.51.100.7, 10.0.0.5, 10.0.0.6", "10.0.0.1:1", "198.51.100.7"},
		"с пробелами":    {"  198.51.100.7  ", "10.0.0.1:1", "198.51.100.7"},
	}
	for name, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = c.remote
		if c.xff != "" {
			req.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := clientIP(req); got != c.want {
			t.Errorf("%s: clientIP = %q, want %q", name, got, c.want)
		}
	}
}

// Паника в обработчике не должна ронять процесс и обязана дать клиенту 500.
func TestObservabilityRecoversPanic(t *testing.T) {
	h := withObservability(slog.New(slog.DiscardHandler), time.Second,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("что-то пошло не так")
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/products", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("код ответа %d, ожидали 500", rec.Code)
	}
	// Клиенту не должно уехать содержимое паники.
	if strings.Contains(rec.Body.String(), "что-то пошло не так") {
		t.Error("текст паники утёк клиенту")
	}
}

func TestObservabilityAddsRequestID(t *testing.T) {
	var seen string
	h := withObservability(slog.New(slog.DiscardHandler), time.Second,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = RequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if seen == "" {
		t.Fatal("request_id не попал в контекст")
	}
	if got := rec.Header().Get("X-Request-Id"); got != seen {
		t.Errorf("заголовок X-Request-Id = %q, в контексте %q", got, seen)
	}
}

func TestClipBoundsInput(t *testing.T) {
	if got := clip("  привет  ", 100); got != "привет" {
		t.Errorf("clip не обрезал пробелы: %q", got)
	}
	long := strings.Repeat("я", 500)
	if got := clip(long, 10); len([]rune(got)) != 10 {
		t.Errorf("clip вернул %d рун, ожидали 10", len([]rune(got)))
	}
	// Обрезка по рунам, а не по байтам: кириллица не должна ломаться.
	if got := clip("привет", 3); got != "при" {
		t.Errorf("clip = %q, ожидали «при»", got)
	}
}
