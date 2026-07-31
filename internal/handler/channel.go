package handler

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Интеграция с каналом магазина (env CHANNEL_ID: @username или -100…):
// /post <id> собирает черновик поста с кнопкой-deep-link в Mini App,
// админ подтверждает — бот публикует в канал. Плюс проверка подписки
// для промокода в Mini App (getChatMember).

// parseChannelID разбирает CHANNEL_ID: @username публичного канала
// или числовой chat_id (приватный канал).
func parseChannelID(raw string) (chatID int64, username string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, ""
	}
	if strings.HasPrefix(raw, "@") {
		return 0, raw
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		log.Printf("CHANNEL_ID %q не распознан (нужен @username или числовой id)", raw)
		return 0, ""
	}
	return id, ""
}

// ChannelEnabled — задан ли канал (включает /post и промокод за подписку).
func (b *Bot) ChannelEnabled() bool {
	return b.channelChatID != 0 || b.channelUsername != ""
}

// ChannelURL — публичная ссылка на канал (пустая для приватного по числовому id).
func (b *Bot) ChannelURL() string {
	if b.channelUsername != "" {
		return "https://t.me/" + strings.TrimPrefix(b.channelUsername, "@")
	}
	return ""
}

// IsSubscribed — состоит ли пользователь в канале (для промокода за подписку).
func (b *Bot) IsSubscribed(userID int64) (bool, error) {
	cfg := tgbotapi.GetChatMemberConfig{
		ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
			ChatID:             b.channelChatID,
			SuperGroupUsername: b.channelUsername,
			UserID:             userID,
		},
	}
	member, err := b.api.GetChatMember(cfg)
	if err != nil {
		// «user not found» и подобное — просто не подписан.
		if permErr, ok := err.(*tgbotapi.Error); ok && permErr.Code == 400 {
			return false, nil
		}
		return false, err
	}
	switch member.Status {
	case "creator", "administrator", "member":
		return true, nil
	}
	return false, nil
}

// productDeepLink — ссылка t.me/<bot>?startapp=product_<id>: открывает Mini App
// сразу на карточке товара (обрабатывается фронтендом по start_param).
func (b *Bot) productDeepLink(productID uint) string {
	return fmt.Sprintf("https://t.me/%s?startapp=product_%d", b.api.Self.UserName, productID)
}

// buildProductPost — фото, подпись и кнопка «Заказать» для поста о товаре.
func (b *Bot) buildProductPost(p *model.Product) (photoURL, caption string, kb tgbotapi.InlineKeyboardMarkup) {
	if len(p.Images) > 0 && b.appURL != "" {
		photoURL = b.appURL + p.Images[0].URL
	}
	minPrice := 0
	for _, v := range p.Variants {
		if minPrice == 0 || v.Price < minPrice {
			minPrice = v.Price
		}
	}
	desc := strings.TrimSpace(p.Description)
	if r := []rune(desc); len(r) > 300 { // caption у Telegram ограничен 1024 символами
		desc = string(r[:300]) + "…"
	}
	if desc != "" {
		desc += "\n"
	}
	caption = fmt.Sprintf(model.ChannelPostCaption, p.Name, desc, minPrice)
	kb = tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonURL(model.ChannelOrderButton, b.productDeepLink(p.ID))))
	return photoURL, caption, kb
}

// handlePostCommand — /post <product_id>: черновик поста с предпросмотром.
func (b *Bot) handlePostCommand(chatID int64, args string) {
	if !b.ChannelEnabled() {
		b.send(chatID, "Канал не настроен: задайте CHANNEL_ID (@username или id канала) и перезапустите сервис.")
		return
	}
	id64, err := strconv.ParseUint(strings.TrimSpace(args), 10, 32)
	if err != nil || id64 == 0 {
		b.send(chatID, "Использование: /post <id товара>\nСписок товаров с id — в /edit.")
		return
	}
	p, err := b.repo.GetProduct(uint(id64))
	if err != nil {
		b.send(chatID, "Товар не найден.")
		return
	}
	if p.IsHidden || p.ArchivedAt != nil {
		b.send(chatID, "Товар скрыт с витрины — сначала откройте его (/hide), иначе ссылка из поста приведёт в никуда.")
		return
	}

	photoURL, caption, orderKb := b.buildProductPost(p)
	// Предпросмотр: тот же пост + строка управления «Опубликовать / Отмена».
	rows := append(orderKb.InlineKeyboard, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅ Опубликовать в канал", fmt.Sprintf("chpub:%d", p.ID)),
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "x"),
	))
	previewKb := tgbotapi.NewInlineKeyboardMarkup(rows...)

	if photoURL == "" {
		msg := tgbotapi.NewMessage(chatID, "Черновик поста (без фото — TELEGRAM_APP_URL не задан или у товара нет фото):\n\n"+caption)
		msg.ReplyMarkup = previewKb
		if m, err := b.tgSend(msg); err == nil {
			b.remember(chatID, m.MessageID)
		}
		return
	}
	photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileURL(photoURL))
	photo.Caption = caption
	photo.ReplyMarkup = previewKb
	m, err := b.tgSend(photo)
	if err != nil {
		log.Printf("предпросмотр поста: %v", err)
		b.send(chatID, "Не удалось собрать предпросмотр: "+err.Error())
		return
	}
	b.remember(chatID, m.MessageID)
}

// publishProductPost — подтверждённая публикация в канал (callback chpub:<id>).
func (b *Bot) publishProductPost(chatID int64, productID uint) {
	p, err := b.repo.GetProduct(productID)
	if err != nil {
		b.send(chatID, "Товар не найден — пост не опубликован.")
		return
	}
	photoURL, caption, orderKb := b.buildProductPost(p)

	var msg tgbotapi.Chattable
	if photoURL == "" {
		m := tgbotapi.NewMessage(b.channelChatID, caption)
		m.ChannelUsername = b.channelUsername
		m.ReplyMarkup = orderKb
		msg = m
	} else {
		ph := tgbotapi.NewPhoto(b.channelChatID, tgbotapi.FileURL(photoURL))
		ph.ChannelUsername = b.channelUsername
		ph.Caption = caption
		ph.ReplyMarkup = orderKb
		msg = ph
	}
	if _, err := b.tgSend(msg); err != nil {
		log.Printf("публикация в канал: %v", err)
		b.send(chatID, "Не удалось опубликовать: "+err.Error()+"\nПроверьте, что бот добавлен в канал администратором.")
		return
	}
	b.send(chatID, fmt.Sprintf("✅ Пост про «%s» опубликован в канал.", p.Name))
}
