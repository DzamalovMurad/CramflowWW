package handler

import (
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// /stock — наличие товаров. Кнопка переключает is_available: товар мгновенно
// пропадает с витрины Mini App (кэш каталога сбрасывается в репозитории),
// но остаётся в прошлых заказах — они читают позиции, а не каталог.

const stockPerPage = 8

func (b *Bot) sendStock(chatID int64) {
	text, kb, ok := b.buildStock(1)
	if !ok {
		b.send(chatID, text)
		return
	}
	b.sendKb(chatID, text, kb)
}

// renderStockPage — правит сообщение на месте: список наличия не должен
// плодить копии себя после каждого переключения.
func (b *Bot) renderStockPage(chatID int64, msgID, page int) {
	text, kb, ok := b.buildStock(page)
	if !ok {
		b.send(chatID, text)
		return
	}
	b.editKb(chatID, msgID, text, kb)
}

func (b *Bot) buildStock(page int) (string, tgbotapi.InlineKeyboardMarkup, bool) {
	if page < 1 {
		page = 1
	}
	total, err := b.repo.CountLiveProducts()
	if err != nil {
		return "Ошибка: " + err.Error(), tgbotapi.InlineKeyboardMarkup{}, false
	}
	if total == 0 {
		return "Товаров пока нет. Добавьте первый: /add", tgbotapi.InlineKeyboardMarkup{}, false
	}

	pages := int((total + stockPerPage - 1) / stockPerPage)
	if page > pages {
		page = pages
	}
	products, err := b.repo.ListProductsPage(page, stockPerPage)
	if err != nil {
		return "Ошибка: " + err.Error(), tgbotapi.InlineKeyboardMarkup{}, false
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📦 Наличие — %s товаров", model.FormatNumber(total))
	if pages > 1 {
		fmt.Fprintf(&sb, " · стр. %d/%d", page, pages)
	}
	sb.WriteString("\n\nКнопка переключает наличие. Снятый товар сразу исчезает из каталога.\n")

	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range products {
		name := p.Name
		if len([]rune(name)) > 22 {
			name = string([]rune(name)[:21]) + "…"
		}
		// Подпись показывает текущее состояние, нажатие — переключает.
		label := fmt.Sprintf("✅ %s", name)
		if !p.IsAvailable {
			label = fmt.Sprintf("⛔ %s", name)
		}
		if p.IsHidden {
			label += " 🙈" // скрыт сезонно — на витрине его нет в любом случае
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("stk:%d:%d", p.ID, page))))
	}

	var nav []tgbotapi.InlineKeyboardButton
	if page > 1 {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("← Назад", fmt.Sprintf("stkpg:%d", page-1)))
	}
	if page < pages {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("Вперёд →", fmt.Sprintf("stkpg:%d", page+1)))
	}
	if len(nav) > 0 {
		rows = append(rows, nav)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
		tgbotapi.NewInlineKeyboardButtonData("🧹 Очистить чат", "clean"),
	))

	return strings.TrimRight(sb.String(), "\n"), tgbotapi.NewInlineKeyboardMarkup(rows...), true
}

// toggleStock — переключение наличия товара и перерисовка той же страницы.
func (b *Bot) toggleStock(chatID int64, msgID int, productID uint, page int) {
	p, err := b.repo.GetProduct(productID)
	if err != nil {
		b.send(chatID, "Товар не найден.")
		return
	}
	if err := b.repo.SetProductAvailable(productID, !p.IsAvailable); err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	b.renderStockPage(chatID, msgID, page)
}
