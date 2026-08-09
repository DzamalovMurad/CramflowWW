package handler

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Режим webhook — основной: Telegram сам будит сервис входящим запросом,
// нет постоянного исходящего соединения и нет «залипания» long polling
// после редеплоя. Long polling остаётся запасным вариантом для локальной
// разработки, где публичного адреса нет.

// maxWebhookBody — апдейт Telegram не бывает больше; всё остальное — мусор.
const maxWebhookBody = 1 << 20

// NewWebhookSecret генерирует секрет, если он не задан в окружении.
func NewWebhookSecret() (string, error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("генерация секрета webhook: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// WebhookPath — секретный путь, куда Telegram присылает апдейты.
func (b *Bot) WebhookPath(secret string) string {
	return "/telegram/" + secret
}

// SetupWebhook регистрирует webhook в Telegram.
//
// Защита двойная: секрет в пути (адрес знают только мы и Telegram) и
// secret_token в заголовке X-Telegram-Bot-Api-Secret-Token, который мы
// сверяем на каждом запросе. Поля secret_token в tgbotapi v5.5.1 нет,
// поэтому setWebhook вызывается напрямую.
func (b *Bot) SetupWebhook(baseURL, secret string) error {
	if baseURL == "" {
		return fmt.Errorf("webhook: не задан публичный адрес сервиса")
	}
	body, err := json.Marshal(map[string]any{
		"url":             baseURL + b.WebhookPath(secret),
		"secret_token":    secret,
		"max_connections": 20,
		"allowed_updates": []string{"message", "callback_query"},
		// Апдейты, накопившиеся у старого бота или за время простоя,
		// обрабатывать незачем: заказы уже в БД, а команды устарели.
		"drop_pending_updates": true,
	})
	if err != nil {
		return err
	}
	if err := b.rawAPI("setWebhook", body); err != nil {
		return fmt.Errorf("webhook: %w", err)
	}
	b.log.Info("бот работает через webhook", "base_url", baseURL)
	return nil
}

// RemoveWebhook снимает webhook (нужно перед возвратом на long polling).
func (b *Bot) RemoveWebhook() error {
	return b.request(tgbotapi.DeleteWebhookConfig{DropPendingUpdates: false})
}

// WebhookHandler принимает апдейты Telegram и отправляет их в тот же
// маршрутизатор, что и long polling.
func (b *Bot) WebhookHandler(secret string) http.HandlerFunc {
	want := []byte(secret)
	return func(w http.ResponseWriter, r *http.Request) {
		got := []byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))
		if subtle.ConstantTimeCompare(got, want) != 1 {
			b.log.Warn("webhook: неверный secret_token", "ip", clientIP(r))
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		var update tgbotapi.Update
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWebhookBody)).Decode(&update); err != nil {
			b.log.Warn("webhook: не удалось разобрать апдейт", "err", err)
			w.WriteHeader(http.StatusOK) // не просим Telegram повторять битый апдейт
			return
		}

		// Отвечаем сразу: Telegram не должен ждать нашу логику, иначе он
		// считает webhook медленным и начинает повторять апдейты.
		w.WriteHeader(http.StatusOK)
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.dispatch(update)
		}()
	}
}
