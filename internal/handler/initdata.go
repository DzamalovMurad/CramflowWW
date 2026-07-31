package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
)

// telegramUserID валидирует initData из Telegram WebApp (HMAC-SHA256 по схеме из
// документации Telegram) и возвращает telegram_id пользователя.
// Возвращает 0, если initData пуста или подпись не сошлась — заказ при этом
// привязывается к общему «гостевому» пользователю (см. README).
func telegramUserID(initData, botToken string) int64 {
	id, _ := telegramLaunch(initData, botToken)
	return id
}

// telegramLaunch — telegram_id пользователя и start_param запуска Mini App
// (t.me/bot?startapp=<param>) из той же валидированной initData.
// Оба значения нулевые при пустой или неподписанной initData.
func telegramLaunch(initData, botToken string) (int64, string) {
	if initData == "" || botToken == "" {
		return 0, ""
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return 0, ""
	}
	gotHash := values.Get("hash")
	if gotHash == "" {
		return 0, ""
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+values.Get(k))
	}
	dataCheckString := strings.Join(pairs, "\n")

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(dataCheckString))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(gotHash)) {
		return 0, ""
	}

	var user struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil {
		return 0, ""
	}
	return user.ID, values.Get("start_param")
}
