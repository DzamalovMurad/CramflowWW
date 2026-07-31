package handler

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Режим webhook нужен на хостингах, которые засыпают без входящего трафика
// (бесплатные тарифы PaaS): при long polling уснувший сервис перестаёт получать
// сообщения, а с webhook входящий запрос от Telegram сам будит контейнер.

// headerSecretToken — заголовок, которым Telegram подтверждает, что апдейт
// действительно от него (Bot API 6.1+).
const headerSecretToken = "X-Telegram-Bot-Api-Secret-Token"

// WebhookPath — секретный путь, куда Telegram присылает апдейты.
func (b *Bot) WebhookPath(secret string) string {
	return "/telegram/" + secret
}

// SetupWebhook регистрирует webhook в Telegram. baseURL — публичный https-адрес
// сервиса, secret — случайная строка: она же в пути, она же в secret_token.
//
// tgbotapi v5.5.1 не умеет secret_token, поэтому setWebhook зовём напрямую:
// без него любой, кто угадает путь, мог бы слать боту поддельные апдейты.
func (b *Bot) SetupWebhook(baseURL, secret string) error {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("webhook: не задан публичный адрес сервиса")
	}
	// Хостинг может отдавать адрес без схемы (например, Render: flowix.onrender.com).
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}

	body, err := json.Marshal(map[string]any{
		"url":             baseURL + b.WebhookPath(secret),
		"max_connections": 20,
		"secret_token":    secret,
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/setWebhook", b.api.Token)
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if !result.OK {
		return fmt.Errorf("webhook: Telegram отказал: %s", result.Description)
	}

	log.Printf("бот работает через webhook: %s%s (secret_token включён)", baseURL, b.WebhookPath(secret))
	return nil
}

// RemoveWebhook снимает webhook (нужно перед возвратом на long polling).
func (b *Bot) RemoveWebhook() error {
	_, err := b.tgRequest(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false})
	return err
}

// WebhookHandler обрабатывает апдейты от Telegram и передаёт их в тот же
// роутер, что и при long polling. Подлинность апдейта подтверждают секрет
// в пути и secret_token в заголовке; права админа всё равно проверяются
// в handleMessage/handleCallback.
func (b *Bot) WebhookHandler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Сравнение постоянного времени: путь и заголовок — общий секрет.
		got := r.Header.Get(headerSecretToken)
		if subtle.ConstantTimeCompare([]byte(got), []byte(secret)) != 1 {
			log.Printf("webhook: отклонён апдейт с неверным secret_token (%s)", clientIP(r))
			http.NotFound(w, r) // не подсказываем, что путь угадан
			return
		}

		update, err := b.api.HandleUpdate(r)
		if err != nil {
			log.Printf("webhook: %v", err)
			w.WriteHeader(http.StatusOK) // не просим Telegram повторять битый апдейт
			return
		}
		// Отвечаем сразу, обработка — в фоне: Telegram не ждёт нашу логику.
		w.WriteHeader(http.StatusOK)

		// Задача учитывается в WaitGroup: при SIGTERM сервис дождётся,
		// пока начатые ответы клиенту уйдут в Telegram.
		if !b.trackStart() {
			return // уже останавливаемся — апдейт Telegram пришлёт повторно
		}
		go func() {
			defer b.trackDone()
			defer func() {
				if rec := recover(); rec != nil {
					b.capturePanic(rec, "webhook")
				}
			}()
			b.dispatch(*update)
		}()
	}
}
