package handler

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// Healthchecker — зависимость, которую /healthz проверяет.
type Healthchecker interface {
	// PingDB должен вернуть ошибку, если БД недоступна.
	PingDB(ctx context.Context) error
	// PingBot проверяет доступность Bot API (nil, если бот не настроен).
	PingBot(ctx context.Context) error
	// SchemaVersion — текущая версия миграций (0, если неизвестна).
	SchemaVersion() int64
}

// botPingTTL — как часто реально ходим в Telegram. Railway дёргает healthcheck
// каждые несколько секунд; getMe на каждый такой запрос быстро упёрся бы
// в лимиты Bot API, поэтому результат кэшируем.
const botPingTTL = 60 * time.Second

type botPingCache struct {
	mu   sync.Mutex
	at   time.Time
	err  error
	done bool
}

func (c *botPingCache) check(ctx context.Context, ping func(context.Context) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done && time.Since(c.at) < botPingTTL {
		return c.err
	}
	c.err = ping(ctx)
	c.at = time.Now()
	c.done = true
	return c.err
}

// healthz — проба для Railway: живы ли БД и Bot API.
//
// БД проверяется всегда: сервис без базы не может ни отдать каталог, ни принять
// заказ. Бот — только если он настроен, и мягко: недоступность Telegram не повод
// перезапускать контейнер, витрина при этом работает. Поэтому такой случай
// отдаёт 200 со статусом degraded, а 503 остаётся за отказом БД.
func (a *API) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	resp := map[string]any{
		"status":  "ok",
		"db":      "ok",
		"bot":     "skipped", // бот не настроен (чистый API)
		"version": a.Health.SchemaVersion(),
	}
	code := http.StatusOK

	if err := a.Health.PingDB(ctx); err != nil {
		resp["status"] = "error"
		resp["db"] = err.Error()
		code = http.StatusServiceUnavailable
	}

	if a.BotToken != "" {
		if err := a.botPing.check(ctx, a.Health.PingBot); err != nil {
			resp["bot"] = err.Error()
			if code == http.StatusOK {
				resp["status"] = "degraded"
			}
		} else {
			resp["bot"] = "ok"
		}
	}

	writeJSON(w, code, resp)
}
