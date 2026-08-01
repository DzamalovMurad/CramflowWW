package handler

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Первый экран Flowix: одно сообщение, одна кнопка, ни одной развилки.
// Клиент попал сюда из рекламы или по ссылке от друга и решает за пару секунд,
// открывать каталог или закрыть чат. Поэтому здесь нет ни описания компании,
// ни списка возможностей — только цветы, эмоция, доставка и действие.

const (
	// welcomeCTA — единственное действие на первом экране.
	welcomeCTA = "🌸 Открыть каталог"

	// captionLimit — предел подписи к фото в Telegram: 1024 символа против
	// 4096 у обычного сообщения.
	captionLimit = 1000
)

// welcomeText — приветствие. firstName может быть пустым (у клиента скрыто имя).
func welcomeText(firstName string) string {
	name := strings.TrimSpace(firstName)
	opening := "Это <b>Flowix</b> 🤍"
	if name != "" {
		opening = fmt.Sprintf("%s, это <b>Flowix</b> 🤍", tgbotapi.EscapeText(tgbotapi.ModeHTML, name))
	}
	return opening + "\n\n" +
		"Букеты, которые говорят за вас.\n\n" +
		"Собираем утром из свежих цветов и привозим по Москве в тот же день — " +
		"экспрессом в течение часа.\n\n" +
		"Загляните в каталог 👇"
}

// promoWelcomeText — тот же первый экран для клиента, пришедшего по ссылке
// с промокодом: подарок сверху, действие то же самое.
func promoWelcomeText(firstName, code, discount, minimum string) string {
	return welcomeText(firstName) + "\n\n" +
		fmt.Sprintf("🎁 Промокод <b>%s</b> уже ваш — скидка %s%s. Применится сам при оформлении.",
			tgbotapi.EscapeText(tgbotapi.ModeHTML, code), discount, minimum)
}

// sendWelcome отправляет первый экран: фото с подписью, если оно настроено,
// иначе тот же текст сообщением. Кнопка одинаковая в обоих случаях, поэтому
// путь Start → кнопка → Mini App не зависит от того, доехало ли фото.
func (b *Bot) sendWelcome(chatID int64, text string) {
	kb, ok := b.shopKeyboard()
	if !ok {
		// Без публичного адреса кнопку web_app собрать не из чего.
		b.send(chatID, stripHTML(text))
		return
	}

	if photo := strings.TrimSpace(b.cfg.WelcomePhoto); photo != "" {
		msg := tgbotapi.NewPhoto(chatID, welcomeFile(photo))
		msg.Caption = clipRunes(text, captionLimit)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.ReplyMarkup = kb
		_, err := b.sendRetry(msg)
		if err == nil {
			return
		}
		// Неверный file_id или недоступная картинка не должны стоить нам
		// клиента: откатываемся на текстовый вариант.
		b.log.Warn("не удалось отправить приветственное фото", "err", err)
	}

	msg := tgbotapi.NewMessage(chatID, clipTelegram(text))
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = kb
	if _, err := b.sendRetry(msg); err != nil {
		b.log.Error("не удалось отправить приветствие", "chat_id", chatID, "err", err)
	}
}

// welcomeFile понимает и https-ссылку, и file_id уже загруженного в Telegram
// снимка. file_id быстрее: Telegram не скачивает картинку заново.
func welcomeFile(photo string) tgbotapi.RequestFileData {
	if strings.HasPrefix(photo, "http://") || strings.HasPrefix(photo, "https://") {
		return tgbotapi.FileURL(photo)
	}
	return tgbotapi.FileID(photo)
}

// shopKeyboard — клавиатура из одной кнопки, открывающей Mini App.
func (b *Bot) shopKeyboard() (webAppKeyboard, bool) {
	if b.cfg.PublicURL == "" {
		return webAppKeyboard{}, false
	}
	btn := webAppButton{Text: welcomeCTA}
	btn.WebApp.URL = b.cfg.PublicURL
	return webAppKeyboard{InlineKeyboard: [][]webAppButton{{btn}}}, true
}

// stripHTML убирает разметку для случая, когда сообщение уходит без ParseMode.
func stripHTML(s string) string {
	r := strings.NewReplacer("<b>", "", "</b>", "", "&amp;", "&", "&lt;", "<", "&gt;", ">")
	return r.Replace(s)
}

func clipRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "…"
}
