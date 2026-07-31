// Package ratelimit — token bucket на клиента (telegram id или IP).
//
// Задача скромная: не дать одному пользователю (или скрипту, дёргающему
// публичный API) выжать пул из 8 соединений к Postgres. Поэтому лимиты
// щедрые для живого человека и жёсткие для цикла в 100 запросов.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter — набор корзин по ключу с фоновой уборкой неактивных.
type Limiter struct {
	rps   rate.Limit
	burst int
	ttl   time.Duration

	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	lim  *rate.Limiter
	seen time.Time
}

// New создаёт лимитер: rps токенов в секунду, burst — размер корзины.
func New(rps float64, burst int) *Limiter {
	return &Limiter{
		rps:     rate.Limit(rps),
		burst:   burst,
		ttl:     10 * time.Minute,
		buckets: make(map[string]*bucket),
	}
}

// Allow списывает токен у ключа. false — клиенту пора притормозить.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[key] = b
		// Уборка по ходу дела: отдельная горутина ради пары сотен ключей
		// не нужна, а карта не должна расти бесконечно.
		if len(l.buckets) > 1024 {
			l.evictLocked()
		}
	}
	b.seen = time.Now()
	return b.lim.Allow()
}

// evictLocked выбрасывает корзины, к которым давно не обращались.
func (l *Limiter) evictLocked() {
	cutoff := time.Now().Add(-l.ttl)
	for k, b := range l.buckets {
		if b.seen.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}
