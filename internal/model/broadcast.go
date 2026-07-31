package model

import "time"

// Рассылка живёт в БД целиком: сама задача (текст/фото, сегмент, статус) и
// поимённый список адресатов с личным статусом каждого. Такое разделение даёт
// два обязательных свойства: рассылку можно продолжить после перезапуска
// процесса и нельзя отправить одному человеку дважды — см. BroadcastRecipient.

// Сегменты аудитории (/broadcast).
const (
	SegmentAll         = "all"           // все, кто запускал бота
	SegmentBuyers      = "buyers"        // есть хотя бы один доставленный заказ
	SegmentCartNoOrder = "cart_no_order" // трогали корзину за 14 дней, но не купили
)

var SegmentLabels = map[string]string{
	SegmentAll:         "Все",
	SegmentBuyers:      "Покупали",
	SegmentCartNoOrder: "Корзина без заказа",
}

var SegmentOrder = []string{SegmentAll, SegmentBuyers, SegmentCartNoOrder}

// CartSegmentDays — окно активности корзины для сегмента «корзина без заказа».
const CartSegmentDays = 14

// Статусы рассылки: draft → queued → running → done (или cancelled).
const (
	BroadcastDraft     = "draft"     // админ ещё набирает контент
	BroadcastQueued    = "queued"    // подтверждена, адресаты зафиксированы
	BroadcastRunning   = "running"   // отправляется прямо сейчас
	BroadcastDone      = "done"      // все адресаты обработаны
	BroadcastCancelled = "cancelled" // админ отменил до старта
)

// Статусы адресата. pending → sending → sent | failed | blocked.
//
// Промежуточный sending — это и есть защита от двойной отправки: строка
// переводится в него ДО вызова Telegram. Если процесс упадёт между отправкой
// и записью результата, при следующем запуске такая строка не попадёт в
// повторную выборку (берём только pending), а будет закрыта как failed —
// лучше не доставить одному человеку, чем прислать ему письмо дважды.
const (
	RecipientPending = "pending"
	RecipientSending = "sending"
	RecipientSent    = "sent"
	RecipientFailed  = "failed"
	RecipientBlocked = "blocked" // пользователь заблокировал бота
)

type Broadcast struct {
	ID      uint   `gorm:"primaryKey" json:"id"`
	AdminID int64  `gorm:"not null" json:"admin_id"` // telegram_id автора
	Segment string `gorm:"not null" json:"segment"`
	Text    string `json:"text"`     // текст или подпись к фото
	PhotoID string `json:"photo_id"` // file_id фото в Telegram (пусто = текстовая)
	Status  string `gorm:"not null;default:draft;index" json:"status"`
	// Чат и сообщение с прогрессом — чтобы дорисовать «Отправлено 120/450»
	// в то же сообщение и после перезапуска процесса.
	ProgressChatID int64      `json:"progress_chat_id"`
	ProgressMsgID  int        `json:"progress_msg_id"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
}

type BroadcastRecipient struct {
	// ID входит в индекс idx_bcast_recipient_status третьим полем: отправщик
	// берёт следующих pending в порядке id, и без него Postgres к концу большой
	// рассылки перебирает всё уже отправленное, прежде чем найти оставшихся.
	ID          uint `gorm:"primaryKey;index:idx_bcast_recipient_status,priority:3" json:"id"`
	BroadcastID uint `gorm:"not null;uniqueIndex:idx_bcast_recipient_unique,priority:1;index:idx_bcast_recipient_status,priority:1" json:"broadcast_id"`
	// Один адресат встречается в рассылке ровно один раз — это гарантирует
	// уникальный индекс, а не аккуратность кода, который наполняет таблицу.
	UserID     uint       `gorm:"not null;uniqueIndex:idx_bcast_recipient_unique,priority:2" json:"user_id"`
	TelegramID int64      `gorm:"not null" json:"telegram_id"`
	Status     string     `gorm:"not null;default:pending;index:idx_bcast_recipient_status,priority:2" json:"status"`
	Error      string     `json:"error,omitempty"`
	SentAt     *time.Time `json:"sent_at,omitempty"`
}

// BroadcastProgress — счётчики рассылки. Всегда считаются из таблицы адресатов
// одним GROUP BY, а не инкрементами в памяти: после перезапуска они обязаны
// сходиться с реальностью.
type BroadcastProgress struct {
	Total   int64
	Pending int64
	Sent    int64
	Failed  int64
	Blocked int64
}

// Done — сколько адресатов уже обработано (в любом исходе).
func (p BroadcastProgress) Done() int64 { return p.Sent + p.Failed + p.Blocked }
