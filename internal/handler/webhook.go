package handler

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Режим webhook нужен на хостингах, которые засыпают без входящего трафика
// (бесплатные тарифы PaaS): при long polling уснувший сервис перестаёт получать
// сообщения, а с webhook входящий запрос от Telegram сам будит контейнер.

// WebhookPath — секретный путь, куда Telegram присылает апдейты.
func (b *Bot) WebhookPath(secret string) string {
	return "/telegram/" + secret
}

// SetupWebhook регистрирует webhook в Telegram. baseURL — публичный https-адрес
// сервиса, secret — случайная строка в пути (плюс secret_token в заголовке).
func (b *Bot) SetupWebhook(baseURL, secret string) error {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return fmt.Errorf("webhook: не задан публичный адрес сервиса")
	}
	// Хостинг может отдавать адрес без схемы (например, Render: flowix.onrender.com).
	if !strings.Contains(baseURL, "://") {
		baseURL = "https://" + baseURL
	}
	wh, err := tgbotapi.NewWebhook(baseURL + b.WebhookPath(secret))
	if err != nil {
		return err
	}
	// tgbotapi v5.5.1 не умеет secret_token, поэтому секрет живёт в пути:
	// адрес знают только Telegram и мы, запросы идут по HTTPS.
	wh.MaxConnections = 20
	if _, err := b.api.Request(wh); err != nil {
		return err
	}
	log.Printf("бот работает через webhook: %s%s", baseURL, b.WebhookPath(secret))
	return nil
}

// RemoveWebhook снимает webhook (нужно перед возвратом на long polling).
func (b *Bot) RemoveWebhook() error {
	_, err := b.api.Request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false})
	return err
}

// WebhookHandler обрабатывает апдейты от Telegram и передаёт их в тот же
// роутер, что и при long polling. Доступ к эндпоинту защищён секретом в пути;
// права админа всё равно проверяются в handleMessage/handleCallback.
func (b *Bot) WebhookHandler(secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		update, err := b.api.HandleUpdate(r)
		if err != nil {
			log.Printf("webhook: %v", err)
			w.WriteHeader(http.StatusOK) // не просим Telegram повторять битый апдейт
			return
		}
		// Отвечаем сразу, обработка — в фоне: Telegram не ждёт нашу логику.
		w.WriteHeader(http.StatusOK)
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("bot panic: %v", rec)
				}
			}()
			switch {
			case update.CallbackQuery != nil:
				b.handleCallback(update.CallbackQuery)
			case update.Message != nil:
				b.handleMessage(update.Message)
			}
		}()
	}
}
