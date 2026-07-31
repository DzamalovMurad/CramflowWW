package handler

import (
	"errors"
	"log"
	"net"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Все обращения к Bot API идут через tgSend/tgRequest: глобальный лимитер
// (~28 сообщений/с при лимите Telegram 30/с) выстраивает вызовы в очередь,
// а 429 обрабатывается повтором с ожиданием retry_after / экспоненциальным бэкоффом.

const (
	tgSendInterval = 35 * time.Millisecond // ≈28 msg/s — с запасом до лимита 30/s
	tgMaxAttempts  = 4
	tgBaseBackoff  = 2 * time.Second // 2s → 4s → 8s, если Telegram не назвал retry_after
)

// tgLimiter — leaky bucket: каждый вызов wait() бронирует следующий слот
// отправки; при всплеске вызовы засыпают до своего слота (очередь по времени).
type tgLimiter struct {
	mu   sync.Mutex
	next time.Time
}

func (l *tgLimiter) wait() {
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	sleep := l.next.Sub(now)
	l.next = l.next.Add(tgSendInterval)
	l.mu.Unlock()
	if sleep > 0 {
		time.Sleep(sleep)
	}
}

// Send — тот же tgSend, но экспортированный: через него ходят внешние
// подсистемы (бэкапы), чтобы их отправки тоже стояли в общей очереди.
func (b *Bot) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) { return b.tgSend(c) }

// tgSend — отправка через лимитер с повтором на 429 (Send: сообщения, фото, правки).
func (b *Bot) tgSend(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	var msg tgbotapi.Message
	err := b.withRetry(func() error {
		var e error
		msg, e = b.api.Send(c)
		return e
	})
	return msg, err
}

// tgRequest — то же для вызовов без сообщения в ответе (delete, callback ack).
func (b *Bot) tgRequest(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	var resp *tgbotapi.APIResponse
	err := b.withRetry(func() error {
		var e error
		resp, e = b.api.Request(c)
		return e
	})
	return resp, err
}

func (b *Bot) withRetry(call func() error) error {
	backoff := tgBaseBackoff
	var err error
	for attempt := 1; attempt <= tgMaxAttempts; attempt++ {
		b.limiter.wait()
		err = call()
		if err == nil {
			return nil
		}
		retryAfter, ok := retryAfterOn429(err)
		if !ok || attempt == tgMaxAttempts {
			return err
		}
		wait := backoff
		if retryAfter > 0 {
			wait = retryAfter
		}
		log.Printf("telegram 429, повтор через %s (попытка %d/%d)", wait, attempt, tgMaxAttempts)
		time.Sleep(wait)
		backoff *= 2
	}
	return err
}

// retryAfterOn429 — если ошибка это flood-limit Telegram, возвращает рекомендованную паузу.
func retryAfterOn429(err error) (time.Duration, bool) {
	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) && tgErr.Code == 429 {
		return time.Duration(tgErr.RetryAfter) * time.Second, true
	}
	return 0, false
}

// isPermanentSendErr — ошибка, которую повторная отправка не исправит:
// клиент заблокировал бота, чат не найден, некорректный запрос.
// Сетевые сбои и 5xx Telegram — временные, их имеет смысл повторить.
func isPermanentSendErr(err error) bool {
	if err == nil {
		return false
	}
	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) {
		return tgErr.Code == 400 || tgErr.Code == 403
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return false
	}
	// Неопознанная ошибка: считаем временной, пусть планировщик попробует ещё раз.
	return false
}
