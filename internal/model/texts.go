// Все тексты, которые бот пишет клиентам и в канал, — в одном файле.
// Правьте здесь, не трогая логику в handler/service.
package model

import (
	"fmt"
	"strings"
	"time"
)

// --- Уведомления о смене статуса ---

// ClientStatusMessages — что бот пишет клиенту при смене статуса (нет ключа = не писать).
// Каждый шаблон обязан содержать ровно один %d — номер заказа (см. ClientStatusText).
var ClientStatusMessages = map[string]string{
	StatusConfirmed:  "Заказ #%d принят ✅",
	StatusAssembling: "Собираем ваш букет 💐 Заказ #%d",
	StatusDelivering: "Курьер в пути 🚗 Заказ #%d",
	StatusDelivered:  "Заказ #%d доставлен. Спасибо! 🌸",
	StatusCancelled:  "Заказ #%d отменён. Если это ошибка — напишите нам.",
}

// ClientDeliveringGift — «Курьер в пути» для подарка (открытка или анонимная доставка):
// получатель может не знать о сюрпризе, поэтому текст адресован дарителю.
const ClientDeliveringGift = "Курьер в пути 🚗 Заказ #%d — скоро вручим ваш сюрприз получателю 🎁"

// ClientCancelledReason — отмена с причиной.
const ClientCancelledReason = "Заказ #%d отменён (%s). Если это ошибка — напишите нам."

// ClientStatusText — текст уведомления клиенту о смене статуса.
// Второе значение false, если для статуса писать не нужно.
// Номер подставляется только при наличии %d в шаблоне — иначе в сообщение
// попадал бы мусор вида "%!(EXTRA uint=4)".
func ClientStatusText(status string, orderID uint) (string, bool) {
	tmpl, ok := ClientStatusMessages[status]
	if !ok || tmpl == "" {
		return "", false
	}
	if !strings.Contains(tmpl, "%d") {
		return tmpl, true
	}
	return fmt.Sprintf(tmpl, orderID), true
}

// ClientOrderStatusText — то же, но с учётом контекста заказа:
// подарочный вариант «Курьер в пути» и причина отмены.
func ClientOrderStatusText(o *Order) (string, bool) {
	if o.Status == StatusDelivering && (o.CardText != "" || o.IsAnonymous) {
		return fmt.Sprintf(ClientDeliveringGift, o.ID), true
	}
	if o.Status == StatusCancelled && o.CancelReason != "" {
		return fmt.Sprintf(ClientCancelledReason, o.ID, o.CancelReason), true
	}
	return ClientStatusText(o.Status, o.ID)
}

// --- Фото букета ---

// Подписи к фото, которое админ шлёт клиенту. Ключ — тип фото (PhotoAssembled/PhotoDelivered).
var PhotoCaptions = map[string]string{
	PhotoAssembled: "Ваш букет собран 🌸 и скоро поедет к вам. Заказ #%d",
	PhotoDelivered: "Доставлен 🎉 Заказ #%d. Пусть радует!",
}

// PhotoCaption — подпись к фото по типу (второе значение false для неизвестного типа).
func PhotoCaption(photoType string, orderID uint) (string, bool) {
	tmpl, ok := PhotoCaptions[photoType]
	if !ok {
		return "", false
	}
	return fmt.Sprintf(tmpl, orderID), true
}

// --- Отзыв после доставки ---

const (
	// FeedbackDelay — через сколько после доставки спрашиваем про букет.
	FeedbackDelay = 2 * time.Hour
	// FeedbackWindow — сколько ждём ответа клиента после вопроса.
	FeedbackWindow = 48 * time.Hour
	// FeedbackPromoPercent — скидка промокода за отзыв.
	FeedbackPromoPercent = 10
	// SubscriptionPromoPercent — скидка промокода за подписку на канал.
	SubscriptionPromoPercent = 5
)

const MsgFeedbackAsk = "Как вам букет? Ответьте на это сообщение или напишите пару слов — подарим промокод на следующий заказ 🌸"

// MsgFeedbackThanks — ответ на отзыв: %d — процент скидки, %s — код.
const MsgFeedbackThanks = "Спасибо за отзыв! 🌸 Дарим промокод на скидку %d%% на следующий заказ: %s\nВведите его при оформлении — скидка применится автоматически."

// MsgFeedbackMore — ответ на повторные сообщения в окне отзыва (промокод уже выдан).
const MsgFeedbackMore = "Спасибо, передали флористу! 🌸"

// AdminFeedbackHeader — заголовок пересылки отзыва админам: %d — номер заказа.
const AdminFeedbackHeader = "⭐ Отзыв по заказу #%d"

// --- Пост в канал ---

// ChannelPostCaption — подпись поста: %s — название, %s — состав/описание, %d — цена «от».
const ChannelPostCaption = "💐 %s\n\n%s\nОт %d ₽ · доставка по Москве"

const ChannelOrderButton = "🌸 Заказать"

// --- Источники трафика (startapp=...) ---

// SourceFromStartParam нормализует startapp-параметр Mini App в метку источника:
//
//	product_<id> — пост товара в канале (метка сохраняется как есть),
//	src_<tag>    — рекламная метка, сохраняем <tag>,
//	прочее       — как есть (обрезаем до 64 символов).
func SourceFromStartParam(p string) string {
	p = strings.TrimSpace(p)
	if strings.HasPrefix(p, "src_") {
		p = strings.TrimPrefix(p, "src_")
	}
	if r := []rune(p); len(r) > 64 {
		p = string(r[:64])
	}
	return p
}
