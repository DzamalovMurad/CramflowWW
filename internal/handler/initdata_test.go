package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Токен только для тестов — настоящий живёт исключительно в окружении.
const fakeToken = "123456:TEST-TOKEN-NOT-A-REAL-BOT"

// signInitData собирает валидную initData тем же алгоритмом, что и Telegram.
func signInitData(token string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+fields[k])
	}

	secret := hmac.New(sha256.New, []byte("WebAppData"))
	secret.Write([]byte(token))
	mac := hmac.New(sha256.New, secret.Sum(nil))
	mac.Write([]byte(strings.Join(pairs, "\n")))

	values := url.Values{}
	for k, v := range fields {
		values.Set(k, v)
	}
	values.Set("hash", hex.EncodeToString(mac.Sum(nil)))
	return values.Encode()
}

func validFields(now time.Time) map[string]string {
	return map[string]string{
		"query_id":  "AAH_test",
		"user":      `{"id":424242,"first_name":"Иван","last_name":"Петров","username":"ivan"}`,
		"auth_date": strconv.FormatInt(now.Unix(), 10),
	}
}

func TestParseInitDataAcceptsValidSignature(t *testing.T) {
	now := time.Now()
	data := signInitData(fakeToken, validFields(now))

	user, err := ParseInitData(data, fakeToken, 24*time.Hour, now)
	if err != nil {
		t.Fatalf("валидная подпись отклонена: %v", err)
	}
	if user.ID != 424242 {
		t.Errorf("telegram_id = %d, ожидали 424242", user.ID)
	}
	if user.FullName() != "Иван Петров" {
		t.Errorf("имя = %q", user.FullName())
	}
}

// Подпись выводится из токена бота: со сменой бота старые initData
// перестают проходить проверку автоматически.
func TestParseInitDataRejectsOtherBotToken(t *testing.T) {
	now := time.Now()
	data := signInitData("999999:OLD-BOT-TOKEN", validFields(now))

	if _, err := ParseInitData(data, fakeToken, 24*time.Hour, now); !errors.Is(err, ErrBadInitData) {
		t.Fatalf("initData чужого бота должна отклоняться, получили %v", err)
	}
}

func TestParseInitDataRejectsTampering(t *testing.T) {
	now := time.Now()
	fields := validFields(now)
	data := signInitData(fakeToken, fields)

	// Подменяем id пользователя, оставляя исходный hash.
	values, _ := url.ParseQuery(data)
	values.Set("user", `{"id":1,"first_name":"Взломщик"}`)

	if _, err := ParseInitData(values.Encode(), fakeToken, 24*time.Hour, now); !errors.Is(err, ErrBadInitData) {
		t.Fatalf("подменённые данные должны отклоняться, получили %v", err)
	}
}

func TestParseInitDataRejectsStale(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	data := signInitData(fakeToken, validFields(old))

	if _, err := ParseInitData(data, fakeToken, 24*time.Hour, now); !errors.Is(err, ErrStaleInitData) {
		t.Fatalf("устаревшая initData должна отклоняться как stale, получили %v", err)
	}
	// С отключённым TTL — принимается.
	if _, err := ParseInitData(data, fakeToken, 0, now); err != nil {
		t.Fatalf("без TTL подпись должна проходить: %v", err)
	}
}

func TestParseInitDataRejectsGarbage(t *testing.T) {
	now := time.Now()
	cases := map[string]string{
		"пусто":               "",
		"без hash":            "user=%7B%22id%22%3A1%7D&auth_date=1",
		"мусорный hash":       "user=%7B%22id%22%3A1%7D&auth_date=1&hash=deadbeef",
		"не query-строка":     "%%%%",
		"hash без остального": "hash=abc",
	}
	for name, data := range cases {
		if _, err := ParseInitData(data, fakeToken, 24*time.Hour, now); err == nil {
			t.Errorf("%s: ожидали ошибку", name)
		}
	}
}

// Без токена подпись проверить нечем — доверять такому запросу нельзя.
func TestParseInitDataRequiresToken(t *testing.T) {
	now := time.Now()
	data := signInitData(fakeToken, validFields(now))
	if _, err := ParseInitData(data, "", 24*time.Hour, now); !errors.Is(err, ErrBadInitData) {
		t.Fatalf("без токена бота initData не должна приниматься, получили %v", err)
	}
}

// Поле signature (сторонняя валидация Ed25519) не участвует в HMAC —
// если его учитывать, подпись Telegram перестаёт сходиться.
func TestParseInitDataIgnoresSignatureField(t *testing.T) {
	now := time.Now()
	fields := validFields(now)
	data := signInitData(fakeToken, fields)

	values, _ := url.ParseQuery(data)
	values.Set("signature", "some-ed25519-signature")

	if _, err := ParseInitData(values.Encode(), fakeToken, 24*time.Hour, now); err != nil {
		t.Fatalf("поле signature не должно ломать проверку: %v", err)
	}
}

func TestFullNameHandlesMissingLastName(t *testing.T) {
	u := TelegramUser{FirstName: "Иван"}
	if u.FullName() != "Иван" {
		t.Errorf("FullName = %q", u.FullName())
	}
	if (TelegramUser{}).FullName() != "" {
		t.Error("пустой пользователь должен давать пустое имя")
	}
}
