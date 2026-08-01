package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

const (
	ordersPerPage  = 5
	maxListedItems = 20
)

// ─── Callback-и заказов ────────────────────────────────────────────────────

func (b *Bot) handleOrderCallback(ctx context.Context, log *slog.Logger, cb *tgbotapi.CallbackQuery, action string, parts []string, argAt func(int) uint) {
	chatID := cb.Message.Chat.ID

	switch action {
	case "pg": // pg:<status>:<page> — страница заказов, правим сообщение на месте
		if len(parts) < 3 {
			return
		}
		b.renderOrderPage(ctx, log, chatID, cb.Message.MessageID, parts[1], int(argAt(2)))

	case "o": // o:<id> — карточка; o:<id>:next — следующий статус; o:<id>:cancel — отмена
		id := argAt(1)
		if len(parts) < 3 {
			o, err := b.repo.GetOrder(ctx, id)
			if err != nil {
				b.adminError(log, chatID, "get order", err)
				return
			}
			kb := adminOrderKeyboard(o)
			b.sendKb(chatID, formatOrder(o, b.cfg.Today(), false), &kb)
			return
		}
		switch parts[2] {
		case "next":
			b.advanceOrder(ctx, log, chatID, cb.Message.MessageID, id, cb.From.ID)
		case "cancel":
			b.setWizard(chatID, &wizard{mode: "cancel_reason", orderID: id, msgID: cb.Message.MessageID})
			reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("Причина отмены заказа #%d (коротко):", id))
			reply.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, InputFieldPlaceholder: "например: клиент передумал"}
			if _, err := b.sendRetry(reply); err != nil {
				log.Error("не удалось запросить причину отмены", "err", err)
			}
		}

	case "cl": // cl:<userID> — карточка клиента со статистикой
		b.sendClientDetails(ctx, log, chatID, argAt(1))

	case "flt": // назад к фильтрам — правим то же сообщение
		text, kb := b.buildStatusFilter(ctx)
		b.editKb(chatID, cb.Message.MessageID, text, kb)

	case "flt2": // фильтры новым сообщением (из финальной карточки)
		b.sendStatusFilter(ctx, chatID)

	case "ophoto": // фото готового букета → клиенту
		id := argAt(1)
		o, err := b.repo.GetOrder(ctx, id)
		if err != nil {
			b.adminError(log, chatID, "get order", err)
			return
		}
		b.setWizard(chatID, &wizard{mode: "order_photo", orderID: id})
		b.send(chatID, fmt.Sprintf("📷 Фото букета для заказа #%d — отправлю его клиенту.\n\n%s\n\n/cancel — отмена.",
			o.ID, sendAsFileHint))
	}
}

// advanceOrder двигает заказ на следующий шаг конвейера и правит карточку на месте.
func (b *Bot) advanceOrder(ctx context.Context, log *slog.Logger, chatID int64, msgID int, orderID uint, adminID int64) {
	o, err := b.repo.GetOrder(ctx, orderID)
	if err != nil {
		b.adminError(log, chatID, "get order", err)
		return
	}
	next := model.NextStatus(o.Status)
	if next == "" {
		b.send(chatID, "Заказ уже в финальном статусе.")
		return
	}
	updated, err := b.svc.TransitionOrder(ctx, orderID, next, adminID, "")
	if err != nil {
		b.adminError(log, chatID, "transition order", err)
		return
	}
	b.editOrderCard(chatID, msgID, updated)
	b.notifyCustomerStatus(ctx, log, updated, next)
}

// finishCancel завершает отмену заказа: причина обязательна.
func (b *Bot) finishCancel(ctx context.Context, log *slog.Logger, msg *tgbotapi.Message, w *wizard, reason string) {
	chatID := msg.Chat.ID
	updated, err := b.svc.TransitionOrder(ctx, w.orderID, model.StatusCancelled, msg.From.ID, reason)
	if err != nil {
		b.adminError(log, chatID, "cancel order", err)
		return
	}
	b.clearWizard(chatID)
	if w.msgID != 0 {
		b.editOrderCard(chatID, w.msgID, updated)
	}
	b.send(chatID, fmt.Sprintf("❌ Заказ #%d отменён: %s", updated.ID, reason))
	b.notifyCustomerStatus(ctx, log, updated, model.StatusCancelled)
}

// ─── Списки заказов ────────────────────────────────────────────────────────

// sendStatusFilter — /orders: фильтр статусов с бейджами-счётчиками.
func (b *Bot) sendStatusFilter(ctx context.Context, chatID int64) {
	text, kb := b.buildStatusFilter(ctx)
	b.sendKb(chatID, text, &kb)
}

func (b *Bot) buildStatusFilter(ctx context.Context) (string, tgbotapi.InlineKeyboardMarkup) {
	counts, err := b.repo.CountOrdersByStatus(ctx)
	if err != nil {
		b.log.Error("не удалось посчитать заказы по статусам", "err", err)
		counts = map[string]int64{}
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, st := range model.StatusOrder {
		label := fmt.Sprintf("%s (%d)", model.StatusLabels[st], counts[st])
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("pg:%s:1", st)))
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, closeRow())

	active := int64(0)
	for _, st := range model.ActiveStatuses {
		active += counts[st]
	}
	return fmt.Sprintf("Заказы по статусам · в работе: %d", active), tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// renderOrderPage правит сообщение со списком заказов статуса (5 на страницу).
func (b *Bot) renderOrderPage(ctx context.Context, log *slog.Logger, chatID int64, msgID int, status string, page int) {
	if _, ok := model.StatusLabels[status]; !ok {
		return
	}
	if page < 1 {
		page = 1
	}
	counts, err := b.repo.CountOrdersByStatus(ctx)
	if err != nil {
		b.adminError(log, chatID, "count orders", err)
		return
	}
	total := int(counts[status])
	pages := (total + ordersPerPage - 1) / ordersPerPage
	orders, err := b.repo.ListOrdersByStatusPage(ctx, status, page, ordersPerPage)
	if err != nil {
		b.adminError(log, chatID, "list orders", err)
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s — %d шт", model.StatusLabels[status], total)
	if pages > 1 {
		fmt.Fprintf(&sb, " · стр. %d/%d", page, pages)
	}
	sb.WriteString("\n\n")

	var rows [][]tgbotapi.InlineKeyboardButton
	if len(orders) == 0 {
		sb.WriteString("Пусто.")
	}
	rows = append(rows, b.orderLines(&sb, orders)...)

	var nav []tgbotapi.InlineKeyboardButton
	if page > 1 {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("← Назад", fmt.Sprintf("pg:%s:%d", status, page-1)))
	}
	nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("Фильтры", "flt"))
	if page < pages {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("Вперёд →", fmt.Sprintf("pg:%s:%d", status, page+1)))
	}
	rows = append(rows, nav, closeRow())

	b.editKb(chatID, msgID, sb.String(), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// sendToday — /today: что везём сегодня, по времени доставки.
// Главный утренний вопрос флориста; раньше на него отвечали перебором фильтров.
func (b *Bot) sendToday(ctx context.Context, chatID int64) {
	today := b.cfg.Today()
	orders, err := b.repo.ListOrdersForDate(ctx, today, maxListedItems)
	if err != nil {
		b.adminError(b.log, chatID, "orders for today", err)
		return
	}
	if len(orders) == 0 {
		b.send(chatID, "На сегодня доставок нет.")
		return
	}
	sum := 0
	for _, o := range orders {
		sum += o.TotalPrice
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "📦 Сегодня, %s — %d заказ(ов) на %d₽\n\n", today, len(orders), sum)
	rows := b.orderLines(&sb, orders)
	rows = append(rows, closeRow())
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendKb(chatID, sb.String(), &kb)
}

// sendPreorders — /preorders: активные заказы на будущие даты.
func (b *Bot) sendPreorders(ctx context.Context, chatID int64) {
	orders, err := b.repo.ListPreorders(ctx, b.cfg.Today(), maxListedItems)
	if err != nil {
		b.adminError(b.log, chatID, "preorders", err)
		return
	}
	if len(orders) == 0 {
		b.send(chatID, "📅 Предзаказов нет — все заказы на сегодня.")
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "📅 Предзаказы (%d):\n\n", len(orders))
	rows := b.orderLines(&sb, orders)
	rows = append(rows, closeRow())
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendKb(chatID, sb.String(), &kb)
}

// orderLines пишет строки списка в буфер и возвращает ряды кнопок «#id».
func (b *Bot) orderLines(sb *strings.Builder, orders []model.Order) [][]tgbotapi.InlineKeyboardButton {
	var rows [][]tgbotapi.InlineKeyboardButton
	var btnRow []tgbotapi.InlineKeyboardButton
	for i := range orders {
		o := &orders[i]
		pre := ""
		if b.isPreorder(o) {
			pre = "📅 "
		}
		fmt.Fprintf(sb, "%s#%d · %s, %s · %s · %d₽ · %s\n",
			pre, o.ID, o.DeliveryDate, o.DeliveryTime, orDash(o.User.Name),
			o.TotalPrice, model.StatusLabels[o.Status])
		btnRow = append(btnRow, tgbotapi.NewInlineKeyboardButtonData(
			fmt.Sprintf("#%d", o.ID), fmt.Sprintf("o:%d", o.ID)))
		if len(btnRow) == 3 {
			rows = append(rows, btnRow)
			btnRow = nil
		}
	}
	if len(btnRow) > 0 {
		rows = append(rows, btnRow)
	}
	return rows
}

// ─── Карточка заказа ───────────────────────────────────────────────────────

// adminOrderKeyboard — кнопки карточки: следующий шаг конвейера, фото, отмена.
func adminOrderKeyboard(o *model.Order) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	if next := model.NextStatus(o.Status); next != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➡️ "+model.StatusLabels[next], fmt.Sprintf("o:%d:next", o.ID))))
	}
	if !model.IsTerminal(o.Status) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📷 Фото букета", fmt.Sprintf("ophoto:%d", o.ID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", fmt.Sprintf("o:%d:cancel", o.ID)),
		))
	}
	if len(rows) == 0 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("К фильтрам", "flt2")))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (b *Bot) editOrderCard(chatID int64, msgID int, o *model.Order) {
	b.editKb(chatID, msgID, formatOrder(o, b.cfg.Today(), false), adminOrderKeyboard(o))
}

// NotifyNewOrder шлёт карточку нового заказа всем админам — основной рабочий поток.
func (b *Bot) NotifyNewOrder(o *model.Order) {
	kb := adminOrderKeyboard(o)
	text := formatOrder(o, b.cfg.Today(), true)
	for _, adminID := range b.cfg.AdminIDs {
		b.sendKb(adminID, text, &kb)
	}
}

func (b *Bot) isPreorder(o *model.Order) bool {
	return o.DeliveryDate > b.cfg.Today()
}

// formatOrder собирает карточку заказа. today — сегодняшняя дата магазина
// (не UTC контейнера): по ней ставится метка предзаказа.
func formatOrder(o *model.Order, today string, isNew bool) string {
	var sb strings.Builder
	pre := ""
	if o.DeliveryDate > today {
		pre = "📅 "
	}
	if isNew {
		fmt.Fprintf(&sb, "🌸 %sНовый заказ #%d\n", pre, o.ID)
	} else {
		fmt.Fprintf(&sb, "🌸 %sЗаказ #%d · %s\n", pre, o.ID, model.StatusLabels[o.Status])
	}
	fmt.Fprintf(&sb, "Заказчик: %s\n", orDash(o.User.Name))
	fmt.Fprintf(&sb, "Телефон: %s\n", orDash(o.User.Phone))
	// Получатель показывается отдельно только если он не заказчик —
	// курьеру важно, кому звонить, особенно при анонимной доставке.
	if o.RecipientName != "" || o.RecipientPhone != "" {
		fmt.Fprintf(&sb, "🎁 Получатель: %s, %s\n", orDash(o.RecipientName), orDash(o.RecipientPhone))
	}

	sb.WriteString("Букеты: ")
	for i, it := range o.Items {
		if i == 4 { // не раздуваем карточку
			fmt.Fprintf(&sb, "; … ещё %d", len(o.Items)-4)
			break
		}
		if i > 0 {
			sb.WriteString("; ")
		}
		fmt.Fprintf(&sb, "%s ×%d шт — %d₽", it.ProductName, it.Variant.Quantity, it.Price)
		if it.Quantity > 1 {
			fmt.Fprintf(&sb, " (×%d = %d₽)", it.Quantity, it.Price*it.Quantity)
		}
	}
	sb.WriteString("\n")

	fmt.Fprintf(&sb, "Адрес: %s\n", o.DeliveryAddress)
	fmt.Fprintf(&sb, "Дата: %s, %s\n", o.DeliveryDate, o.DeliveryTime)
	// Код берём из снимка в заказе: саму акцию могли удалить, а заказ обязан
	// объяснять свою скидку.
	if code := orderPromoCode(o); code != "" {
		fmt.Fprintf(&sb, "Промокод: %s (−%d₽)\n", code, o.DiscountAmount)
	}
	fmt.Fprintf(&sb, "Итого: %d₽\n", o.TotalPrice)
	if o.CardText != "" {
		fmt.Fprintf(&sb, "💌 Открытка: %s\n", o.CardText)
	}
	if o.IsAnonymous {
		sb.WriteString("🤫 Анонимная доставка — не называть отправителя\n")
	}
	if o.Comment != "" {
		fmt.Fprintf(&sb, "Комментарий: %s\n", o.Comment)
	}
	if o.Status == model.StatusCancelled && o.CancelReason != "" {
		fmt.Fprintf(&sb, "Причина отмены: %s\n", o.CancelReason)
	}
	return strings.TrimSpace(sb.String())
}

// ─── Клиенты ───────────────────────────────────────────────────────────────

func (b *Bot) sendClientSearch(ctx context.Context, chatID int64, query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		b.send(chatID, "Использование: /clients <имя или телефон>\nНапример: /clients мурад или /clients 8999")
		return
	}
	if len([]rune(query)) > 64 {
		b.send(chatID, "Слишком длинный запрос.")
		return
	}
	users, err := b.repo.SearchClients(ctx, query, 5)
	if err != nil {
		b.adminError(b.log, chatID, "search clients", err)
		return
	}
	if len(users) == 0 {
		b.send(chatID, "Никого не нашлось по запросу «"+query+"».")
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Найдено %d:\n\n", len(users))
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, u := range users {
		fmt.Fprintf(&sb, "• %s · %s\n", orDash(u.Name), orDash(u.Phone))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				clipLabel("Подробнее: "+orDash(u.Name)), fmt.Sprintf("cl:%d", u.ID))))
	}
	rows = append(rows, closeRow())
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendKb(chatID, sb.String(), &kb)
}

func (b *Bot) sendClientDetails(ctx context.Context, log *slog.Logger, chatID int64, userID uint) {
	u, err := b.repo.GetUserByID(ctx, userID)
	if err != nil {
		b.adminError(log, chatID, "get client", err)
		return
	}
	stats, err := b.repo.GetClientStats(ctx, userID)
	if err != nil {
		b.adminError(log, chatID, "client stats", err)
		return
	}
	last, err := b.repo.LastClientOrders(ctx, userID, 5)
	if err != nil {
		log.Error("не удалось загрузить заказы клиента", "user_id", userID, "err", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "👤 %s\nТелефон: %s\n\n", orDash(u.Name), orDash(u.Phone))
	fmt.Fprintf(&sb, "Заказов: %d (доставлено %d, отменено %d)\n", stats.Orders, stats.Delivered, stats.Cancelled)
	fmt.Fprintf(&sb, "LTV: %d₽\n", stats.LTV)
	fmt.Fprintf(&sb, "Средний чек: %.0f₽\n", stats.AvgCheck)

	if len(last) == 0 {
		b.send(chatID, sb.String())
		return
	}
	sb.WriteString("\nПоследние заказы:\n")
	rows := b.orderLines(&sb, last)
	rows = append(rows, closeRow())
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendKb(chatID, sb.String(), &kb)
}

// ─── Уведомления клиенту ───────────────────────────────────────────────────

// sendBouquetPhoto отправляет фото готового букета клиенту.
// Статус при этом не меняется: отправка фото — событие, а не шаг конвейера.
func (b *Bot) sendBouquetPhoto(ctx context.Context, log *slog.Logger, chatID int64, orderID uint, fileID string) {
	o, err := b.repo.GetOrder(ctx, orderID)
	if err != nil {
		b.adminError(log, chatID, "get order", err)
		return
	}
	if o.User.TelegramID == 0 {
		b.send(chatID, "У клиента нет Telegram ID — фото отправить некому.")
		return
	}
	// Шлём документом, а не фото: sendPhoto пересжал бы снимок и клиент увидел
	// мыло вместо своего букета. Telegram показывает картинку-документ с превью.
	doc := tgbotapi.NewDocument(o.User.TelegramID, tgbotapi.FileID(fileID))
	doc.Caption = fmt.Sprintf("🌸 Ваш букет к заказу #%d готов!", o.ID)
	if _, err := b.sendRetry(doc); err != nil {
		log.Error("не удалось отправить фото клиенту", "order_id", o.ID, "err", err)
		b.send(chatID, "Не удалось отправить фото клиенту (возможно, он не запускал бота).")
		return
	}
	b.send(chatID, fmt.Sprintf("✅ Фото отправлено клиенту заказа #%d.", o.ID))
}

// notifyCustomerStatus сообщает клиенту о смене статуса заказа.
func (b *Bot) notifyCustomerStatus(_ context.Context, log *slog.Logger, o *model.Order, status string) {
	text, ok := model.ClientStatusText(status, o.ID)
	if !ok {
		return
	}
	if o.User.TelegramID == 0 {
		return
	}
	if status == model.StatusCancelled && o.CancelReason != "" {
		text += "\nПричина: " + o.CancelReason
	}
	msg := tgbotapi.NewMessage(o.User.TelegramID, clipTelegram(text))
	if _, err := b.sendRetry(msg); err != nil {
		// Клиент мог заблокировать бота — это не наша ошибка и не повод шуметь.
		log.Info("клиенту не доставлено уведомление о статусе",
			"order_id", o.ID, "status", status, "err", err)
	}
}

// SendBackup отправляет дамп БД администраторам (интерфейс backup.Sender).
// Достаточно доставить хотя бы одному: копия у владельца — уже копия.
func (b *Bot) SendBackup(name string, data []byte, caption string) error {
	if len(b.cfg.AdminIDs) == 0 {
		return fmt.Errorf("некому отправить бэкап: администраторы не заданы")
	}
	var lastErr error
	delivered := 0
	for _, adminID := range b.cfg.AdminIDs {
		doc := tgbotapi.NewDocument(adminID, tgbotapi.FileBytes{Name: name, Bytes: data})
		doc.Caption = caption
		doc.DisableNotification = true // ночью будить владельца незачем
		if _, err := b.sendRetry(doc); err != nil {
			lastErr = err
			continue
		}
		delivered++
	}
	if delivered == 0 {
		return lastErr
	}
	return nil
}
