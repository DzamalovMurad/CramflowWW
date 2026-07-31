// Package observability — Sentry и сквозной request id для логов.
//
// Sentry опционален: без SENTRY_DSN все функции работают как no-op, ошибки
// просто уходят в лог. Это важно для локальной разработки и для деплоя,
// где мониторинг ещё не подключён.
package observability

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/getsentry/sentry-go"
)

var enabled bool

// Init поднимает Sentry. Пустой dsn — тихо выключаем.
func Init(dsn, environment, release string) {
	if dsn == "" {
		log.Println("SENTRY_DSN не задан — трекинг ошибок выключен")
		return
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: environment,
		Release:     release,
		// Пользовательские данные заказа (телефон, адрес) не должны утекать
		// в трекер: шлём только технический контекст.
		SendDefaultPII: false,
		// Трейсинг не включаем — на этом объёме трафика он не окупает шум.
		EnableTracing: false,
	})
	if err != nil {
		log.Printf("sentry: инициализация не удалась: %v", err)
		return
	}
	enabled = true
	log.Printf("sentry подключён (environment=%s)", environment)
}

// Enabled — подключён ли трекер (для /healthz и диагностики).
func Enabled() bool { return enabled }

// Flush досылает накопленные события перед выходом. Вызывается при shutdown.
func Flush(timeout time.Duration) {
	if enabled {
		sentry.Flush(timeout)
	}
}

// CaptureError отправляет ошибку в Sentry и всегда пишет её в лог.
// tags — пары ключ/значение (component, request_id, ...).
func CaptureError(err error, tags map[string]string) {
	if err == nil {
		return
	}
	log.Printf("error: %v (%v)", err, tags)
	if !enabled {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentry.CaptureException(err)
	})
}

// CapturePanic оформляет перехваченную панику как ошибку со стеком.
// Вызывать из recover(): передайте значение recover() и контекст места.
func CapturePanic(rec any, tags map[string]string) {
	err := fmt.Errorf("panic: %v", rec)
	log.Printf("PANIC %v (%v)", rec, tags)
	if !enabled {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		scope.SetLevel(sentry.LevelFatal)
		sentry.CaptureException(err)
	})
}

// GoSafe запускает горутину, у которой паника не роняет процесс, а уезжает
// в Sentry. Фоновых горутин в сервисе много (уведомления, вебхук, бэкапы),
// и каждая обязана иметь свой recover.
func GoSafe(component string, fn func()) {
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				CapturePanic(rec, map[string]string{"component": component})
			}
		}()
		fn()
	}()
}

// --- request id ---

type ctxKey struct{}

// WithRequestID кладёт id запроса в контекст.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// RequestID достаёт id запроса из контекста («-», если его там нет).
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(ctxKey{}).(string); ok && id != "" {
		return id
	}
	return "-"
}
