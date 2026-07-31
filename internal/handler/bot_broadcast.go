package handler

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/observability"
)

// /broadcast — рассылка по сегменту аудитории.
//
// Устойчивость к падениям обеспечивает БД, а не память процесса: контент и
// поимённый список адресатов сохраняются до первой отправки, каждый адресат
// имеет собственный статус, а прогресс всегда считается запросом к таблице.
// После перезапуска ResumeBroadcasts() дошлёт незавершённое, ни разу не
// написав тому, кому уже написали (см. model.RecipientSending).

const (
	// Telegram разрешает ~30 сообщений в секунду; держим 20 с запасом,
	// чтобы рассылка не выедала лимит у оперативных уведомлений о заказах.
	broadcastRate  = 20
	broadcastBatch = 20
	// Как часто дорисовывать «Отправлено 120/450»: edit — тоже запрос к API,
	// и на большой аудитории он бы съел заметную долю лимита.
	progressEvery = 3 * time.Second
)

// --- Шаг 1: выбор сегмента ---

func (b *Bot) sendBroadcastSegments(chatID int64) {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, seg := range model.SegmentOrder {
		n, err := b.repo.CountSegment(seg)
		label := model.SegmentLabels[seg]
		if err == nil {
			label = fmt.Sprintf("%s (%s)", label, model.FormatNumber(n))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "bcseg:"+seg)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x")))

	b.sendKb(chatID, "📣 Кому отправляем?\n\n"+
		"• Все — все, кто запускал бота\n"+
		"• Покупали — есть доставленный заказ\n"+
		fmt.Sprintf("• Корзина без заказа — трогали корзину за %d дней, но не купили",
			model.CartSegmentDays),
		tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// --- Шаг 2: приём контента ---

func (b *Bot) startBroadcastContent(chatID int64, segment string) {
	b.setWizard(chatID, &wizard{mode: "broadcast_content", segment: segment})
	b.send(chatID, fmt.Sprintf("Сегмент: %s.\n\n"+
		"Пришлите текст рассылки или фото с подписью.\n"+
		"Разметка не применяется — что видите, то и получат клиенты.\n\n"+
		"/cancel — отмена.", model.SegmentLabels[segment]))
}

// broadcastContent принимает текст или фото и показывает предпросмотр.
// В отличие от фото товара, сжатое фото здесь уместно: это маркетинговая
// картинка, а не витрина, и Telegram переотправит её по file_id без загрузки.
func (b *Bot) broadcastContent(msg *tgbotapi.Message, w *wizard) {
	chatID := msg.Chat.ID

	var photoID, text string
	switch {
	case len(msg.Photo) > 0:
		photoID = msg.Photo[len(msg.Photo)-1].FileID // последний размер — самый крупный
		text = strings.TrimSpace(msg.Caption)
	case msg.Document != nil && isImageDocument(msg.Document):
		photoID = msg.Document.FileID
		text = strings.TrimSpace(msg.Caption)
	default:
		text = strings.TrimSpace(msg.Text)
	}

	if photoID == "" && text == "" {
		b.send(chatID, "Нужен текст или фото. Пришлите ещё раз или /cancel.")
		return
	}

	count, err := b.repo.CountSegment(w.segment)
	if err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	if count == 0 {
		b.clearWizard(chatID)
		b.send(chatID, "В этом сегменте сейчас никого нет — рассылать некому.")
		return
	}

	// Автор — отправитель сообщения; для служебных апдейтов без From
	// (пересылка от имени канала) остаётся сам чат, он и так админский.
	adminID := chatID
	if msg.From != nil {
		adminID = msg.From.ID
	}

	// Черновик пишем в БД сразу: кнопка подтверждения носит только его номер,
	// поэтому подделать текст рассылки через callback-payload невозможно.
	bc := &model.Broadcast{
		AdminID: adminID,
		Segment: w.segment,
		Text:    text,
		PhotoID: photoID,
		Status:  model.BroadcastDraft,
	}
	if err := b.repo.CreateBroadcast(bc); err != nil {
		b.send(chatID, "Ошибка сохранения: "+err.Error())
		return
	}
	b.clearWizard(chatID)

	b.send(chatID, "👀 Предпросмотр — так это увидит клиент:")
	if err := b.sendBroadcastContent(chatID, bc); err != nil {
		// Если контент не уходит даже админу, рассылать его тем более нельзя.
		b.send(chatID, "Не удалось показать предпросмотр: "+err.Error()+"\nПопробуйте /broadcast заново.")
		return
	}

	kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Отправить ✅", fmt.Sprintf("bc:%d:go", bc.ID)),
		tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", fmt.Sprintf("bc:%d:no", bc.ID)),
	))
	b.sendKb(chatID, fmt.Sprintf("Сегмент: %s\nПолучателей: %s\n\nОтправляем?",
		model.SegmentLabels[bc.Segment], model.FormatNumber(count)), kb)
}

// sendBroadcastContent отправляет контент рассылки одному получателю.
func (b *Bot) sendBroadcastContent(chatID int64, bc *model.Broadcast) error {
	if bc.PhotoID != "" {
		photo := tgbotapi.NewPhoto(chatID, tgbotapi.FileID(bc.PhotoID))
		photo.Caption = bc.Text
		_, err := b.api.Send(photo)
		return err
	}
	_, err := b.api.Send(tgbotapi.NewMessage(chatID, bc.Text))
	return err
}

// --- Шаг 3: подтверждение и отправка ---

func (b *Bot) confirmBroadcast(chatID int64, msgID int, id uint) {
	bc, err := b.repo.GetBroadcast(id)
	if err != nil {
		b.send(chatID, "Рассылка не найдена.")
		return
	}
	if bc.Status != model.BroadcastDraft {
		b.send(chatID, "Эта рассылка уже запущена или отменена.")
		return
	}

	// Фиксируем аудиторию: с этого момента список адресатов заморожен и
	// переживёт перезапуск. Повторное нажатие сюда уже не дойдёт — статус
	// сменится на queued, и вернётся ошибка выше.
	total, err := b.repo.QueueBroadcast(id, bc.Segment)
	if err != nil {
		b.send(chatID, "Не удалось поставить рассылку в очередь: "+err.Error())
		return
	}

	b.editKb(chatID, msgID, fmt.Sprintf("📣 Рассылка №%d запущена.\nПолучателей: %s",
		id, model.FormatNumber(total)), tgbotapi.NewInlineKeyboardMarkup())

	// Сообщение с прогрессом: его номер храним в БД, чтобы после перезапуска
	// дописывать прогресс в него же, а не заводить второе.
	progress, err := b.api.Send(tgbotapi.NewMessage(chatID, "⏳ Отправлено 0/"+model.FormatNumber(total)))
	if err == nil {
		bc.ProgressChatID = chatID
		bc.ProgressMsgID = progress.MessageID
		if err := b.repo.SaveBroadcast(bc); err != nil {
			log.Printf("broadcast: сохранение прогресса: %v", err)
		}
		b.remember(chatID, progress.MessageID)
	}

	observability.GoSafe("bot:broadcast", func() { b.runBroadcast(id) })
}

func (b *Bot) cancelBroadcast(chatID int64, msgID int, id uint) {
	bc, err := b.repo.GetBroadcast(id)
	if err != nil {
		return
	}
	if bc.Status != model.BroadcastDraft {
		b.send(chatID, "Рассылка уже запущена — отменить нельзя.")
		return
	}
	if err := b.repo.FinishBroadcast(id, model.BroadcastCancelled); err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	b.editKb(chatID, msgID, "❌ Рассылка отменена.", tgbotapi.NewInlineKeyboardMarkup())
}

// runBroadcast — отправщик. Рассылки идут строго по одной: их лимит общий
// с оперативными уведомлениями, и две параллельные упёрлись бы в 429.
func (b *Bot) runBroadcast(id uint) {
	b.bcastMu.Lock()
	defer b.bcastMu.Unlock()

	claimed, err := b.repo.ClaimBroadcast(id)
	if err != nil {
		log.Printf("broadcast %d: claim: %v", id, err)
		return
	}
	if !claimed {
		return // уже отправляется, отменена или завершена
	}

	bc, err := b.repo.GetBroadcast(id)
	if err != nil {
		log.Printf("broadcast %d: %v", id, err)
		return
	}

	ticker := time.NewTicker(time.Second / broadcastRate)
	defer ticker.Stop()
	lastProgress := time.Now()

	// Рассылка считается завершённой, только если очередь опустела штатно.
	// Обрыв на ошибке БД оставляет адресатов в pending, и закрывать её нельзя:
	// иначе остаток аудитории молча потеряется.
	completed := false
	// Остановка сервиса — не ошибка: выходим из цикла, остаток дошлёт
	// ResumeBroadcasts после рестарта. Держать деплой все 20 секунд ожидания
	// ради рассылки на тысячу адресатов бессмысленно — она всё равно не успеет.
	interrupted := false

	for {
		if b.stopping.Load() {
			interrupted = true
			break
		}
		batch, err := b.repo.NextRecipients(id, broadcastBatch)
		if err != nil {
			log.Printf("broadcast %d: выборка адресатов: %v", id, err)
			break
		}
		if len(batch) == 0 {
			completed = true
			break
		}

		for _, rcp := range batch {
			if b.stopping.Load() {
				interrupted = true
				break
			}
			<-ticker.C
			status, errMsg := b.deliverBroadcast(bc, rcp)
			if err := b.repo.MarkRecipient(rcp.ID, status, errMsg); err != nil {
				log.Printf("broadcast %d: отметка адресата %d: %v", id, rcp.ID, err)
			}
			if status == model.RecipientBlocked {
				if err := b.repo.MarkUserBlocked(rcp.UserID); err != nil {
					log.Printf("broadcast %d: пометка блокировки: %v", id, err)
				}
			}
		}

		if time.Since(lastProgress) >= progressEvery {
			b.editBroadcastProgress(bc)
			lastProgress = time.Now()
		}
	}

	if !completed {
		// Возвращаем в очередь: остаток дошлём после перезапуска сервиса,
		// уже отправленные адресаты повторно не получат ничего.
		if err := b.repo.RequeueBroadcast(id); err != nil {
			log.Printf("broadcast %d: возврат в очередь: %v", id, err)
		}
		if interrupted {
			// Штатная остановка: писать админу нечего — сообщение с прогрессом
			// уже висит в чате, и ResumeBroadcasts дорисует его после старта.
			log.Printf("broadcast %d: остановка сервиса, остаток дошлём после рестарта", id)
			return
		}
		b.send(broadcastChat(bc), fmt.Sprintf(
			"⚠️ Рассылка №%d прервана из-за ошибки базы. Остаток будет отправлен "+
				"после перезапуска сервиса — те, кто уже получил сообщение, повторно его не увидят.", id))
		return
	}

	b.editBroadcastProgress(bc) // финальные цифры в том же сообщении
	if err := b.repo.FinishBroadcast(id, model.BroadcastDone); err != nil {
		log.Printf("broadcast %d: завершение: %v", id, err)
	}
	b.reportBroadcast(bc)
}

// deliverBroadcast отправляет одно сообщение и классифицирует исход.
func (b *Bot) deliverBroadcast(bc *model.Broadcast, rcp model.BroadcastRecipient) (status, errMsg string) {
	err := b.sendBroadcastContent(rcp.TelegramID, bc)
	if err == nil {
		return model.RecipientSent, ""
	}

	// 429: Telegram просит подождать — это не отказ, повторяем один раз.
	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) && tgErr.Code == 429 {
		wait := time.Duration(tgErr.RetryAfter) * time.Second
		if wait <= 0 || wait > time.Minute {
			wait = time.Second
		}
		time.Sleep(wait)
		if err = b.sendBroadcastContent(rcp.TelegramID, bc); err == nil {
			return model.RecipientSent, ""
		}
	}

	if isBlockedError(err) {
		return model.RecipientBlocked, err.Error()
	}
	return model.RecipientFailed, err.Error()
}

// isBlockedError — клиент заблокировал бота, удалил аккаунт или чат недоступен.
// Такому адресату писать больше нечего: помечаем bot_blocked и пропускаем впредь.
func isBlockedError(err error) bool {
	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) && tgErr.Code == 403 {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"bot was blocked by the user",
		"user is deactivated",
		"chat not found",
		"bot can't initiate conversation",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func (b *Bot) editBroadcastProgress(bc *model.Broadcast) {
	if bc.ProgressMsgID == 0 {
		return
	}
	p, err := b.repo.BroadcastProgress(bc.ID)
	if err != nil {
		return
	}
	text := fmt.Sprintf("⏳ Отправлено %s/%s",
		model.FormatNumber(p.Sent), model.FormatNumber(p.Total))
	edit := tgbotapi.NewEditMessageText(bc.ProgressChatID, bc.ProgressMsgID, text)
	if _, err := b.api.Send(edit); err != nil {
		// «message is not modified» — обычное дело, если за интервал ничего не ушло.
		if !strings.Contains(err.Error(), "not modified") {
			log.Printf("broadcast %d: прогресс: %v", bc.ID, err)
		}
	}
}

// broadcastChat — куда писать о ходе рассылки. Сообщение с прогрессом могло
// не создаться (Telegram ответил ошибкой) — тогда пишем автору напрямую.
func broadcastChat(bc *model.Broadcast) int64 {
	if bc.ProgressChatID != 0 {
		return bc.ProgressChatID
	}
	return bc.AdminID
}

// reportBroadcast — итоговый отчёт автору рассылки.
func (b *Bot) reportBroadcast(bc *model.Broadcast) {
	p, err := b.repo.BroadcastProgress(bc.ID)
	if err != nil {
		return
	}
	chatID := broadcastChat(bc)

	var sb strings.Builder
	fmt.Fprintf(&sb, "✅ Рассылка №%d завершена\n\n", bc.ID)
	fmt.Fprintf(&sb, "Сегмент: %s\n", model.SegmentLabels[bc.Segment])
	fmt.Fprintf(&sb, "Отправлено: %s из %s\n", model.FormatNumber(p.Sent), model.FormatNumber(p.Total))
	fmt.Fprintf(&sb, "Не доставлено: %s\n", model.FormatNumber(p.Failed))
	fmt.Fprintf(&sb, "Заблокировали бота: %s", model.FormatNumber(p.Blocked))
	if p.Blocked > 0 {
		sb.WriteString("\n\nЗаблокировавшие исключены из будущих рассылок.")
	}
	b.send(chatID, sb.String())
}

// ResumeBroadcasts продолжает рассылки, прерванные остановкой сервиса.
// Вызывается один раз при старте бота.
func (b *Bot) ResumeBroadcasts() {
	n, err := b.repo.RecoverInterruptedRecipients()
	if err != nil {
		log.Printf("broadcast: восстановление: %v", err)
		return
	}
	if n > 0 {
		log.Printf("broadcast: %d адресатов закрыто как прерванные (повторно не отправляем)", n)
	}

	pending, err := b.repo.UnfinishedBroadcasts()
	if err != nil {
		log.Printf("broadcast: незавершённые: %v", err)
		return
	}
	for _, bc := range pending {
		log.Printf("broadcast %d: продолжаем после перезапуска", bc.ID)
		id := bc.ID
		observability.GoSafe("bot:broadcast", func() { b.runBroadcast(id) })
	}
}
