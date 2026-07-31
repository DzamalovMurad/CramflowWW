package repository

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// --- Сегменты аудитории ---

// segmentWhere — условие выборки адресатов сегмента для таблицы users (алиас u).
// Одно место для счётчика в предпросмотре и для фиксации списка адресатов:
// иначе «получателей 450» и реально разосланное разъезжаются.
//
// Общее для всех сегментов: у человека есть telegram_id (иначе писать некуда)
// и он не заблокировал бота.
func segmentWhere(segment string) (string, []any) {
	base := "u.telegram_id <> 0 AND u.bot_blocked = FALSE"

	// Покупкой считаем доставленный заказ: оплаченный, но отменённый —
	// это не покупатель.
	hasDelivered := `EXISTS (SELECT 1 FROM orders o
		WHERE o.user_id = u.id AND o.status = ?)`

	switch segment {
	case model.SegmentBuyers:
		return base + " AND " + hasDelivered, []any{model.StatusDelivered}

	case model.SegmentCartNoOrder:
		cutoff := time.Now().AddDate(0, 0, -model.CartSegmentDays)
		return base + " AND u.last_cart_at >= ? AND NOT " + hasDelivered,
			[]any{cutoff, model.StatusDelivered}

	default: // model.SegmentAll
		return base, nil
	}
}

// CountSegment — сколько человек получит рассылку (для предпросмотра).
func (r *Repository) CountSegment(segment string) (int64, error) {
	where, args := segmentWhere(segment)
	var n int64
	err := r.DB.Raw(`SELECT COUNT(*) FROM users u WHERE `+where, args...).Scan(&n).Error
	return n, err
}

// --- Жизненный цикл рассылки ---

func (r *Repository) CreateBroadcast(b *model.Broadcast) error {
	return r.DB.Create(b).Error
}

func (r *Repository) GetBroadcast(id uint) (*model.Broadcast, error) {
	var b model.Broadcast
	if err := r.DB.First(&b, id).Error; err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *Repository) SaveBroadcast(b *model.Broadcast) error {
	return r.DB.Save(b).Error
}

// QueueBroadcast фиксирует список адресатов и переводит рассылку в queued —
// одной транзакцией. С этого момента аудитория заморожена: клиент,
// зарегистрировавшийся во время отправки, в текущую рассылку не попадёт,
// а список можно безопасно доотправить после перезапуска.
//
// Возвращает число адресатов.
func (r *Repository) QueueBroadcast(id uint, segment string) (int64, error) {
	where, args := segmentWhere(segment)
	var n int64

	err := r.DB.Transaction(func(tx *gorm.DB) error {
		// Только из draft: повторное подтверждение той же кнопки (двойной тап,
		// ретрай Telegram) не должно наполнить таблицу второй раз.
		res := tx.Model(&model.Broadcast{}).
			Where("id = ? AND status = ?", id, model.BroadcastDraft).
			Update("status", model.BroadcastQueued)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		// Список адресатов пишем одним INSERT ... SELECT: без выгрузки
		// пользователей в память и без вставки по одному.
		sql := fmt.Sprintf(`
			INSERT INTO broadcast_recipients (broadcast_id, user_id, telegram_id, status)
			SELECT ?, u.id, u.telegram_id, ?
			FROM users u
			WHERE %s
			ON CONFLICT (broadcast_id, user_id) DO NOTHING`, where)

		insertArgs := append([]any{id, model.RecipientPending}, args...)
		ins := tx.Exec(sql, insertArgs...)
		if ins.Error != nil {
			return ins.Error
		}
		n = ins.RowsAffected
		return nil
	})
	return n, err
}

// BroadcastProgress — счётчики из таблицы адресатов одним GROUP BY.
// Считаем всегда от БД, а не от счётчика в памяти: после перезапуска
// процесса «Отправлено 120/450» обязано остаться правдой.
func (r *Repository) BroadcastProgress(id uint) (*model.BroadcastProgress, error) {
	var row struct {
		Total   int64
		Pending int64
		Sent    int64
		Failed  int64
		Blocked int64
	}
	err := r.DB.Raw(`
		SELECT COUNT(*)                                  AS total,
		       COUNT(*) FILTER (WHERE status IN (?, ?))  AS pending,
		       COUNT(*) FILTER (WHERE status = ?)        AS sent,
		       COUNT(*) FILTER (WHERE status = ?)        AS failed,
		       COUNT(*) FILTER (WHERE status = ?)        AS blocked
		FROM broadcast_recipients
		WHERE broadcast_id = ?`,
		model.RecipientPending, model.RecipientSending,
		model.RecipientSent, model.RecipientFailed, model.RecipientBlocked,
		id).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	return &model.BroadcastProgress{
		Total: row.Total, Pending: row.Pending,
		Sent: row.Sent, Failed: row.Failed, Blocked: row.Blocked,
	}, nil
}

// ClaimBroadcast переводит рассылку в running. Возвращает false, если её уже
// кто-то ведёт или она завершена — так вторая горутина (или второй инстанс,
// поднятый рядом) не начнёт слать те же сообщения повторно.
func (r *Repository) ClaimBroadcast(id uint) (bool, error) {
	now := time.Now()
	res := r.DB.Model(&model.Broadcast{}).
		Where("id = ? AND status = ?", id, model.BroadcastQueued).
		Updates(map[string]any{"status": model.BroadcastRunning, "started_at": &now})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// NextRecipients забирает пачку адресатов и сразу помечает их sending.
// Пометка до отправки — обязательная часть защиты от дублей: см. комментарий
// к model.RecipientSending. UPDATE ... RETURNING делает выборку и захват
// атомарными, поэтому две горутины не возьмут одну строку.
func (r *Repository) NextRecipients(broadcastID uint, limit int) ([]model.BroadcastRecipient, error) {
	var out []model.BroadcastRecipient
	err := r.DB.Raw(`
		UPDATE broadcast_recipients SET status = ?
		WHERE id IN (
			SELECT id FROM broadcast_recipients
			WHERE broadcast_id = ? AND status = ?
			ORDER BY id
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`,
		model.RecipientSending, broadcastID, model.RecipientPending, limit).
		Scan(&out).Error
	return out, err
}

// MarkRecipient фиксирует исход отправки одному адресату.
func (r *Repository) MarkRecipient(id uint, status, errMsg string) error {
	now := time.Now()
	upd := map[string]any{"status": status, "error": errMsg}
	if status == model.RecipientSent {
		upd["sent_at"] = &now
	}
	return r.DB.Model(&model.BroadcastRecipient{}).Where("id = ?", id).Updates(upd).Error
}

// MarkUserBlocked — клиент заблокировал бота: исключаем его из будущих рассылок.
func (r *Repository) MarkUserBlocked(userID uint) error {
	return r.DB.Model(&model.User{}).Where("id = ?", userID).
		Update("bot_blocked", true).Error
}

// FinishBroadcast закрывает рассылку.
func (r *Repository) FinishBroadcast(id uint, status string) error {
	now := time.Now()
	return r.DB.Model(&model.Broadcast{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "finished_at": &now}).Error
}

// RequeueBroadcast возвращает рассылку в очередь, не закрывая её: отправка
// оборвалась на ошибке БД, часть адресатов ещё ждёт. finished_at остаётся
// пустым — рассылка не завершена, её подхватит ResumeBroadcasts при старте.
func (r *Repository) RequeueBroadcast(id uint) error {
	return r.DB.Model(&model.Broadcast{}).Where("id = ?", id).
		Update("status", model.BroadcastQueued).Error
}

// --- Восстановление после перезапуска ---

// RecoverInterruptedRecipients закрывает строки, застрявшие в sending:
// процесс упал между вызовом Telegram и записью результата, и достоверно
// неизвестно, ушло сообщение или нет. Такие адресаты помечаются failed и
// повторно НЕ отправляются — молчание безопаснее дубля в личке клиента.
// Возвращает число закрытых строк.
func (r *Repository) RecoverInterruptedRecipients() (int64, error) {
	res := r.DB.Model(&model.BroadcastRecipient{}).
		Where("status = ?", model.RecipientSending).
		Updates(map[string]any{
			"status": model.RecipientFailed,
			"error":  "прервано перезапуском сервиса",
		})
	return res.RowsAffected, res.Error
}

// UnfinishedBroadcasts — рассылки, которые нужно продолжить после старта.
// running возвращаем в queued: прежний отправщик умер вместе с процессом,
// новый обязан пройти через ClaimBroadcast.
func (r *Repository) UnfinishedBroadcasts() ([]model.Broadcast, error) {
	if err := r.DB.Model(&model.Broadcast{}).
		Where("status = ?", model.BroadcastRunning).
		Update("status", model.BroadcastQueued).Error; err != nil {
		return nil, err
	}
	var out []model.Broadcast
	err := r.DB.Where("status = ?", model.BroadcastQueued).Order("id").Find(&out).Error
	return out, err
}

// TouchCart отмечает активность в корзине Mini App (сегмент «корзина без заказа»).
// Пользователя создаём, если его ещё нет: до первого заказа строки в users нет,
// а именно эти люди и есть искомый сегмент «положил в корзину, но не купил».
// Имя и телефон не трогаем — они приходят из checkout и точнее телеграмных.
func (r *Repository) TouchCart(telegramID int64) error {
	if telegramID == 0 {
		return nil
	}
	u, err := r.UpsertUser(telegramID, "", "")
	if err != nil {
		return err
	}
	now := time.Now()
	return r.DB.Model(&model.User{}).Where("id = ?", u.ID).
		Update("last_cart_at", &now).Error
}
