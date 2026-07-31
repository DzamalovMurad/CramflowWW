package handler

import (
	"fmt"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

// /stats — сводка за период. Отчёт собирается тремя агрегатными запросами
// (см. repository/stats.go), кнопки периодов правят то же сообщение.

const (
	topProductsLimit = 3
	// Каналы мельче 5% заказов и всё, что не влезло в четвёрку крупнейших,
	// сворачиваются в «другое»: длинный хвост меток не помогает решать.
	sourcesKeep     = 4
	sourcesMinShare = 0.05
)

func (b *Bot) sendStats(chatID int64) {
	text, kb := b.buildStats(repository.PeriodToday)
	b.sendKb(chatID, text, kb)
}

// statsKeyboard — переключатель периодов, выбранный помечен точкой.
func statsKeyboard(active string) tgbotapi.InlineKeyboardMarkup {
	var row []tgbotapi.InlineKeyboardButton
	for _, p := range repository.PeriodOrder {
		label := repository.PeriodLabels[p]
		if p == active {
			label = "• " + label
		}
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(label, "st:"+p))
	}
	return tgbotapi.NewInlineKeyboardMarkup(
		row,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", "st:"+active),
			tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
		),
	)
}

func (b *Bot) buildStats(period string) (string, tgbotapi.InlineKeyboardMarkup) {
	from := repository.PeriodStart(period, time.Now())

	summary, err := b.repo.GetStatsSummary(from)
	if err != nil {
		return "Не удалось собрать отчёт: " + err.Error(), statsKeyboard(period)
	}
	top, err := b.repo.GetTopProducts(from, topProductsLimit)
	if err != nil {
		return "Не удалось собрать отчёт: " + err.Error(), statsKeyboard(period)
	}
	sources, err := b.repo.GetSourceStats(from)
	if err != nil {
		return "Не удалось собрать отчёт: " + err.Error(), statsKeyboard(period)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📊 %s\n", repository.PeriodLabels[period])
	fmt.Fprintf(&sb, "с %s\n\n", from.Format("02.01 15:04"))

	fmt.Fprintf(&sb, "🌸 Доставлено: %s заказов на %s\n",
		model.FormatNumber(summary.DoneOrders), model.FormatMoney(summary.DoneRevenue))
	fmt.Fprintf(&sb, "⏳ В работе: %s заказов на %s\n",
		model.FormatNumber(summary.ActiveOrders), model.FormatMoney(summary.ActiveRevenue))
	fmt.Fprintf(&sb, "💳 Средний чек: %s\n", model.FormatMoney(int64(summary.AvgCheck+0.5)))
	fmt.Fprintf(&sb, "❌ Отменено: %s\n", model.FormatNumber(summary.Cancelled))
	fmt.Fprintf(&sb, "👤 Новых клиентов: %s\n", model.FormatNumber(summary.NewUsers))

	sb.WriteString("\n🏆 Топ букетов:\n")
	if len(top) == 0 {
		sb.WriteString("— пока нечего показать\n")
	}
	for i, p := range top {
		fmt.Fprintf(&sb, "%d. %s — %s шт (%s)\n",
			i+1, p.Name, model.FormatNumber(p.Qty), model.FormatMoney(p.Revenue))
	}

	sb.WriteString("\n📣 Откуда заказы:\n")
	grouped := repository.GroupSmallSources(sources, sourcesKeep, sourcesMinShare)
	if len(grouped) == 0 {
		sb.WriteString("— заказов за период нет\n")
	}
	for _, s := range grouped {
		fmt.Fprintf(&sb, "• %s — %s (%s)\n",
			model.SourceLabel(s.Source), model.FormatNumber(s.Orders), model.FormatMoney(s.Revenue))
	}

	return strings.TrimRight(sb.String(), "\n"), statsKeyboard(period)
}
