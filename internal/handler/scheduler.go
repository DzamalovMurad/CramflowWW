package handler

import (
	"fmt"
	"log"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Планировщик отложенных уведомлений: простой in-process тикер раз в минуту.
// Очередь живёт в таблице notifications (due_at), отметка sent_at ставится
// атомарно до отправки — перезапуски не теряют и не дублируют сообщения.

const schedulerBatch = 50

// RunScheduler — запускать горутиной из main. Первый проход сразу:
// после простоя/перезапуска накопившиеся уведомления уходят без ожидания тика.
func (b *Bot) RunScheduler() {
	log.Println("планировщик уведомлений запущен (тик раз в минуту)")
	b.processDueNotifications()
	for range time.Tick(time.Minute) {
		b.processDueNotifications()
	}
}

func (b *Bot) processDueNotifications() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("scheduler panic: %v", r)
		}
	}()
	due, err := b.repo.DueNotifications(time.Now(), schedulerBatch)
	if err != nil {
		log.Printf("выборка уведомлений: %v", err)
		return
	}
	for _, n := range due {
		// Сначала помечаем отправленным (guard от дублей при гонке),
		// при временной ошибке отправки возвращаем в очередь.
		claimed, err := b.repo.ClaimNotification(n.ID, time.Now())
		if err != nil || !claimed {
			continue
		}
		if err := b.deliverNotification(&n); err != nil {
			if isPermanentSendErr(err) {
				log.Printf("уведомление #%d по заказу #%d не доставлено (навсегда): %v", n.ID, n.OrderID, err)
				continue
			}
			log.Printf("уведомление #%d по заказу #%d отложено до следующего тика: %v", n.ID, n.OrderID, err)
			if err := b.repo.UnclaimNotification(n.ID); err != nil {
				log.Printf("возврат уведомления #%d в очередь: %v", n.ID, err)
			}
		}
	}
}

// deliverNotification — отправка одного уведомления по типу.
// nil и для «отправлять некому/незачем»: такие помечены sent и не повторяются.
func (b *Bot) deliverNotification(n *model.Notification) error {
	switch n.Type {
	case model.NotificationFeedback:
		o, err := b.repo.GetOrder(n.OrderID)
		if err != nil {
			return fmt.Errorf("заказ не найден: %w", err)
		}
		// Заказ успели отменить или клиент вне Telegram — вопрос не задаём.
		if o.Status != model.StatusDelivered || o.User.TelegramID == 0 {
			return nil
		}
		_, err = b.tgSend(tgbotapi.NewMessage(o.User.TelegramID, model.MsgFeedbackAsk))
		return err
	default:
		log.Printf("неизвестный тип уведомления %q (#%d) — пропущен", n.Type, n.ID)
		return nil
	}
}
