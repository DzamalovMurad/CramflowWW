package model

import "strings"

// Каналы привлечения. Значение приходит из start_param Mini App
// (t.me/bot/app?startapp=instagram) или из ?src= в ссылке на сайт.
const (
	SourceDirect    = "direct"
	SourceInstagram = "instagram"
	SourceTelegram  = "telegram"
	SourceVK        = "vk"
	SourceYandex    = "yandex"
	SourceAvito     = "avito"
	SourceOffline   = "offline"
	SourceReferral  = "referral"
	SourceOther     = "другое" // служебная строка отчёта, в БД не пишется
)

// SourceLabels — подписи для отчёта /stats. Незнакомый источник показывается
// как есть: маркетолог заводит метку в ссылке, а не в коде.
var SourceLabels = map[string]string{
	SourceDirect:    "Напрямую",
	SourceInstagram: "Instagram",
	SourceTelegram:  "Telegram",
	SourceVK:        "ВКонтакте",
	SourceYandex:    "Яндекс",
	SourceAvito:     "Авито",
	SourceOffline:   "Офлайн",
	SourceReferral:  "Рекомендации",
}

// SourceLabel — подпись источника для отчёта.
func SourceLabel(s string) string {
	if label, ok := SourceLabels[s]; ok {
		return label
	}
	if s == "" {
		return SourceLabels[SourceDirect]
	}
	return s
}

// maxSourceLen — метка длиннее не нужна и не должна раздувать индекс.
const maxSourceLen = 32

// NormalizeSource приводит метку канала к безопасному виду: нижний регистр,
// только латиница/цифры/дефис/подчёркивание. Значение приходит от клиента,
// поэтому мусор и попытки подсунуть SQL/разметку отсекаются здесь, а не в SQL.
// Пустое или полностью отфильтрованное значение — SourceDirect.
func NormalizeSource(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= maxSourceLen {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), "-_")
	if out == "" {
		return SourceDirect
	}
	return out
}
