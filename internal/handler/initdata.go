package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrNoInitData — заголовок отсутствует или пуст.
	ErrNoInitData = errors.New("initData не передана")
	// ErrBadInitData — подпись не сошлась или данные испорчены.
	ErrBadInitData = errors.New("подпись initData неверна")
	// ErrStaleInitData — подпись верна, но слишком старая (защита от повтора).
	ErrStaleInitData = errors.New("сессия Telegram устарела")
)

// TelegramUser — то, что нам нужно из initData.
type TelegramUser struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

// FullName — имя для предзаполнения формы.
func (u TelegramUser) FullName() string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}

// ParseInitData валидирует initData Telegram WebApp по схеме из документации:
// secret = HMAC_SHA256("WebAppData", bot_token), затем HMAC_SHA256(secret, data_check_string).
//
// Ключ подписи выводится из токена бота — при смене бота старые initData
// перестают проходить проверку автоматически, отдельной миграции не требуется.
func ParseInitData(initData, botToken string, ttl time.Duration, now time.Time) (*TelegramUser, error) {
	if initData == "" {
		return nil, ErrNoInitData
	}
	if botToken == "" {
		return nil, ErrBadInitData
	}
	values, err := url.ParseQuery(initData)
	if err != nil {
		return nil, ErrBadInitData
	}
	gotHash := values.Get("hash")
	if gotHash == "" {
		return nil, ErrBadInitData
	}

	keys := make([]string, 0, len(values))
	for k := range values {
		// signature — поле сторонней валидации (Ed25519), в HMAC не участвует.
		if k != "hash" && k != "signature" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+values.Get(k))
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(botToken))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(pairs, "\n")))
	expected := hex.EncodeToString(mac.Sum(nil))

	if subtle.ConstantTimeCompare([]byte(expected), []byte(gotHash)) != 1 {
		return nil, ErrBadInitData
	}

	// auth_date защищает от повторного использования утёкшей initData.
	if ttl > 0 {
		ts, err := strconv.ParseInt(values.Get("auth_date"), 10, 64)
		if err != nil {
			return nil, ErrBadInitData
		}
		if now.Sub(time.Unix(ts, 0)) > ttl {
			return nil, ErrStaleInitData
		}
	}

	var user TelegramUser
	if err := json.Unmarshal([]byte(values.Get("user")), &user); err != nil || user.ID == 0 {
		return nil, ErrBadInitData
	}
	return &user, nil
}
