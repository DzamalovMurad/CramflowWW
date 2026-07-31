// Package service — бизнес-логика: приём заказа, деньги, промокоды, статусы.
//
// Инварианты, которые здесь держатся:
//   - суммы считаются только по ценам из БД, клиентские цифры игнорируются;
//   - заказ, списание остатков и списание промокода происходят в одной транзакции;
//   - повторная отправка той же формы (Idempotency-Key) не создаёт второй заказ.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dzamalovmurad/cramflowww/internal/config"
	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

// Пределы длины пользовательских строк. Каждое поле обязано иметь границу:
// без неё один запрос кладёт в БД мегабайт и распухает карточка в Telegram.
const (
	maxNameLen      = 80
	maxPhoneDigits  = 15
	minPhoneDigits  = 10
	maxAddressLen   = 300
	maxCommentLen   = 500
	maxCardTextLen  = 300
	maxPromoCodeLen = 32
	maxCartLines    = 20
	maxLineQty      = 99
	// Максимальный горизонт предзаказа.
	maxPreorderDays = 60
)

// ExpressDelivery — доставка «в течение часа» (только на сегодня).
const ExpressDelivery = "в течение часа"

var deliveryAtRe = regexp.MustCompile(`^к ([0-2]\d):([0-5]\d)$`)

// ─── Ошибки ────────────────────────────────────────────────────────────────

// ValidationError — ошибка, текст которой можно показать клиенту как есть.
type ValidationError struct {
	Msg string
	// UnavailableVariants — позиции корзины, которых больше нет. Фронтенд
	// убирает именно их, а не заставляет пересобирать всю корзину.
	UnavailableVariants []uint
}

func (e *ValidationError) Error() string { return e.Msg }

func invalid(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// ─── Сервис ────────────────────────────────────────────────────────────────

type Service struct {
	Repo *repository.Repository
	Cfg  *config.Config
	Log  *slog.Logger

	// NotifyNewOrder вызывается после коммита заказа (бот шлёт карточку админам).
	// Сбой уведомления не должен ломать оформление — вызывается вне транзакции.
	NotifyNewOrder func(o *model.Order)

	// nowFn — часы магазина. Отдельным полем, чтобы тесты правил доставки
	// не зависели от того, в какое время суток их запустили.
	nowFn func() time.Time

	// notifying считает незавершённые уведомления, чтобы редеплой не оборвал
	// отправку карточки нового заказа флористу.
	notifying sync.WaitGroup
}

// DrainNotifications ждёт, пока разойдутся уведомления о заказах.
// Вызывается при остановке сервиса: заказ уже в БД, но флорист должен
// получить карточку, а не узнавать о заказе из /orders постфактум.
func (s *Service) DrainNotifications(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		s.notifying.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		s.Log.Warn("уведомления о заказах не успели отправиться до остановки")
	}
}

func New(repo *repository.Repository, cfg *config.Config, log *slog.Logger) *Service {
	return &Service{Repo: repo, Cfg: cfg, Log: log, nowFn: cfg.Now}
}

func (s *Service) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return s.Cfg.Now()
}

type OrderItemInput struct {
	VariantID uint `json:"variant_id"`
	Quantity  int  `json:"quantity"`
}

type OrderInput struct {
	Items           []OrderItemInput `json:"items"`
	Name            string           `json:"name"`
	Phone           string           `json:"phone"`
	DeliveryAddress string           `json:"delivery_address"`
	DeliveryDate    string           `json:"delivery_date"`
	DeliveryTime    string           `json:"delivery_time"`
	RecipientName   string           `json:"recipient_name"`
	RecipientPhone  string           `json:"recipient_phone"`
	Comment         string           `json:"comment"`
	CardText        string           `json:"card_text"`
	IsAnonymous     bool             `json:"is_anonymous"`
	PromoCode       string           `json:"promo_code"`

	// Заполняются хендлером, не клиентом.
	TelegramID     int64  `json:"-"`
	IdempotencyKey string `json:"-"`
}

// CreateOrder — единственный путь появления заказа в системе.
func (s *Service) CreateOrder(ctx context.Context, in OrderInput) (*model.Order, error) {
	if in.TelegramID == 0 {
		return nil, invalid("не удалось определить пользователя Telegram — откройте магазин заново")
	}

	clean, err := s.validate(in)
	if err != nil {
		return nil, err
	}

	// Повтор той же формы: возвращаем уже созданный заказ, а не второй такой же.
	if clean.IdempotencyKey != "" {
		if existing, err := s.Repo.FindOrderByIdempotencyKey(ctx, clean.IdempotencyKey); err == nil {
			return s.Repo.GetOrder(ctx, existing.ID)
		} else if !errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("проверка идемпотентности: %w", err)
		}
	}

	var orderID uint
	err = s.Repo.Tx(ctx, func(tx *repository.Repository) error {
		user, err := tx.UpsertUser(ctx, clean.TelegramID, clean.Name, clean.Phone)
		if err != nil {
			return err
		}

		items, subtotal, err := s.buildItems(ctx, tx, clean.Items)
		if err != nil {
			return err
		}

		promo, err := s.resolvePromo(ctx, tx, user, clean.PromoCode)
		if err != nil {
			return err
		}

		discount, total := 0, subtotal
		var promoID *uint
		if promo != nil {
			discount, total = model.ApplyDiscount(subtotal, promo.DiscountPercent)
			promoID = &promo.ID
		}

		order := &model.Order{
			UserID:          user.ID,
			SubtotalPrice:   subtotal,
			DiscountAmount:  discount,
			TotalPrice:      total,
			DeliveryAddress: clean.DeliveryAddress,
			DeliveryDate:    clean.DeliveryDate,
			DeliveryTime:    clean.DeliveryTime,
			RecipientName:   clean.RecipientName,
			RecipientPhone:  clean.RecipientPhone,
			PromoCodeID:     promoID,
			Comment:         clean.Comment,
			CardText:        clean.CardText,
			IsAnonymous:     clean.IsAnonymous,
			Status:          model.StatusNew,
			Items:           items,
		}
		if clean.IdempotencyKey != "" {
			key := clean.IdempotencyKey
			order.IdempotencyKey = &key
		}
		if err := tx.CreateOrder(ctx, order); err != nil {
			return fmt.Errorf("сохранение заказа: %w", err)
		}

		if promo != nil {
			if err := tx.RedeemPromo(ctx, promo.ID, user.ID, order.ID); err != nil {
				return err
			}
		}
		orderID = order.ID
		return nil
	})
	if err != nil {
		// Гонка на ключе идемпотентности: параллельный запрос успел первым.
		if clean.IdempotencyKey != "" && isUniqueViolation(err) {
			if existing, findErr := s.Repo.FindOrderByIdempotencyKey(ctx, clean.IdempotencyKey); findErr == nil {
				return s.Repo.GetOrder(ctx, existing.ID)
			}
		}
		return nil, err
	}

	full, err := s.Repo.GetOrder(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("чтение созданного заказа #%d: %w", orderID, err)
	}
	s.notifyNewOrder(full)
	return full, nil
}

// notifyNewOrder уводит уведомление в фон: Telegram может отвечать секундами,
// а клиент не должен ждать. Паника внутри не должна ронять сервис.
func (s *Service) notifyNewOrder(o *model.Order) {
	if s.NotifyNewOrder == nil {
		return
	}
	s.notifying.Add(1)
	go func() {
		defer s.notifying.Done()
		defer func() {
			if rec := recover(); rec != nil {
				s.Log.Error("паника при уведомлении о заказе", "order_id", o.ID, "panic", rec)
			}
		}()
		s.NotifyNewOrder(o)
	}()
}

// buildItems превращает корзину в позиции заказа, считает сумму по ценам из БД
// и списывает остатки. Всё внутри транзакции заказа.
func (s *Service) buildItems(ctx context.Context, tx *repository.Repository, in []OrderItemInput) ([]model.OrderItem, int, error) {
	ids := make([]uint, 0, len(in))
	for _, it := range in {
		ids = append(ids, it.VariantID)
	}
	rows, err := tx.GetVariantsForOrder(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	byID := make(map[uint]repository.VariantForOrder, len(rows))
	for _, row := range rows {
		byID[row.VariantID] = row
	}

	// Чего не нашлось — то исчезло с витрины, пока корзина лежала у клиента.
	var missing []uint
	for _, it := range in {
		if _, ok := byID[it.VariantID]; !ok {
			missing = append(missing, it.VariantID)
		}
	}
	if len(missing) > 0 {
		return nil, 0, &ValidationError{
			Msg:                 "часть букетов из корзины больше недоступна — мы их убрали, проверьте заказ",
			UnavailableVariants: missing,
		}
	}

	// Сколько штук каждого товара в заказе — остаток ведётся на товаре, не на варианте.
	perProduct := map[uint]int{}
	items := make([]model.OrderItem, 0, len(in))
	subtotal := 0
	for _, it := range in {
		v := byID[it.VariantID]
		subtotal += v.Price * it.Quantity
		perProduct[v.ProductID] += it.Quantity
		items = append(items, model.OrderItem{
			VariantID:   v.VariantID,
			Quantity:    it.Quantity,
			Price:       v.Price,
			ProductName: v.ProductName,
		})
	}

	// Списываем остатки строго по возрастанию id товара. Обход map даёт
	// случайный порядок, и две параллельные транзакции с одними и теми же
	// товарами в разном порядке блокировали бы строки крест-накрест —
	// классический deadlock, который Postgres разрывает откатом заказа.
	productIDs := make([]uint, 0, len(perProduct))
	for id := range perProduct {
		productIDs = append(productIDs, id)
	}
	slices.Sort(productIDs)

	for _, productID := range productIDs {
		if err := tx.DecrementStock(ctx, productID, perProduct[productID]); err != nil {
			if errors.Is(err, repository.ErrOutOfStock) {
				return nil, 0, invalid("остатка не хватает — уменьшите количество или выберите другой букет")
			}
			return nil, 0, err
		}
	}
	return items, subtotal, nil
}

// resolvePromo выбирает и проверяет промокод: явно введённый приоритетнее
// сохранённого по deep-link. Строка кода блокируется до конца транзакции,
// поэтому два одновременных заказа не пробьют лимит применений.
func (s *Service) resolvePromo(ctx context.Context, tx *repository.Repository, user *model.User, code string) (*model.PromoCode, error) {
	var promoID uint
	explicit := code != ""
	if explicit {
		p, err := tx.GetPromoByCode(ctx, code)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return nil, invalid("промокод не найден")
			}
			return nil, fmt.Errorf("промокод: %w", err)
		}
		promoID = p.ID
	} else if user.PromoCodeID != nil {
		promoID = *user.PromoCodeID
	}
	if promoID == 0 {
		return nil, nil
	}

	promo, err := tx.LockPromo(ctx, promoID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) && !explicit {
			return nil, nil // сохранённый код удалили — просто не применяем
		}
		return nil, err
	}

	reject := func(msg string) (*model.PromoCode, error) {
		if explicit {
			return nil, invalid("%s", msg)
		}
		return nil, nil // молча игнорируем протухший deep-link-код
	}

	if !promo.Usable(s.now()) {
		return reject("промокод больше не действует")
	}
	if promo.PerUserLimit > 0 {
		used, err := tx.CountUserRedemptions(ctx, promo.ID, user.ID)
		if err != nil {
			return nil, fmt.Errorf("проверка лимита промокода: %w", err)
		}
		if used >= promo.PerUserLimit {
			return reject("этот промокод уже использован")
		}
	}
	return promo, nil
}

// ─── Валидация ─────────────────────────────────────────────────────────────

func (s *Service) validate(in OrderInput) (OrderInput, error) {
	out := in
	out.Name = collapseSpaces(in.Name)
	out.DeliveryAddress = collapseSpaces(in.DeliveryAddress)
	out.RecipientName = collapseSpaces(in.RecipientName)
	out.Comment = strings.TrimSpace(in.Comment)
	out.CardText = strings.TrimSpace(in.CardText)
	out.DeliveryDate = strings.TrimSpace(in.DeliveryDate)
	out.DeliveryTime = collapseSpaces(in.DeliveryTime)
	out.PromoCode = strings.ToUpper(strings.TrimSpace(in.PromoCode))
	out.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)

	if len(out.Items) == 0 {
		return out, invalid("корзина пуста")
	}
	if len(out.Items) > maxCartLines {
		return out, invalid("слишком много позиций в заказе — оставьте не больше %d", maxCartLines)
	}
	seen := make(map[uint]bool, len(out.Items))
	for _, it := range out.Items {
		if it.VariantID == 0 {
			return out, invalid("некорректная позиция заказа")
		}
		if seen[it.VariantID] {
			return out, invalid("одна и та же позиция передана дважды")
		}
		seen[it.VariantID] = true
		if it.Quantity < 1 || it.Quantity > maxLineQty {
			return out, invalid("количество должно быть от 1 до %d", maxLineQty)
		}
	}

	if err := checkLen("имя", out.Name, 2, maxNameLen); err != nil {
		return out, err
	}
	phone, err := normalizePhone(out.Phone)
	if err != nil {
		return out, err
	}
	out.Phone = phone

	if err := checkLen("адрес доставки", out.DeliveryAddress, 5, maxAddressLen); err != nil {
		return out, err
	}
	if utf8.RuneCountInString(out.Comment) > maxCommentLen {
		return out, invalid("комментарий — не более %d символов", maxCommentLen)
	}
	if utf8.RuneCountInString(out.CardText) > maxCardTextLen {
		return out, invalid("текст открытки — не более %d символов", maxCardTextLen)
	}
	if utf8.RuneCountInString(out.PromoCode) > maxPromoCodeLen {
		return out, invalid("промокод слишком длинный")
	}
	if utf8.RuneCountInString(out.IdempotencyKey) > 64 {
		return out, invalid("некорректный запрос")
	}

	// Получатель необязателен; если указан — валидируем так же строго.
	if out.RecipientName != "" || out.RecipientPhone != "" {
		if err := checkLen("имя получателя", out.RecipientName, 2, maxNameLen); err != nil {
			return out, err
		}
		rp, err := normalizePhone(out.RecipientPhone)
		if err != nil {
			return out, invalid("укажите корректный телефон получателя")
		}
		out.RecipientPhone = rp
	}

	if err := s.validateDelivery(out.DeliveryDate, out.DeliveryTime); err != nil {
		return out, err
	}
	return out, nil
}

// validateDelivery проверяет дату и время доставки относительно часов работы
// магазина и его часового пояса (не UTC контейнера).
func (s *Service) validateDelivery(date, deliveryTime string) error {
	now := s.now()
	day, err := time.ParseInLocation("2006-01-02", date, s.Cfg.Location)
	if err != nil {
		return invalid("выберите дату доставки")
	}
	todayDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, s.Cfg.Location)
	if day.Before(todayDate) {
		return invalid("дата доставки уже прошла — выберите другую")
	}
	if day.After(todayDate.AddDate(0, 0, maxPreorderDays)) {
		return invalid("предзаказ принимаем не более чем на %d дней вперёд", maxPreorderDays)
	}
	isToday := day.Equal(todayDate)

	open := s.Cfg.ShopOpenHour * 60
	closeAt := s.Cfg.ShopCloseHour * 60
	nowMin := now.Hour()*60 + now.Minute()

	if deliveryTime == ExpressDelivery {
		if !isToday {
			return invalid("доставка «%s» возможна только сегодня — для другой даты выберите точное время", ExpressDelivery)
		}
		if nowMin < open || nowMin > closeAt {
			return invalid("сейчас мы не доставляем: приём заказов «%s» с %02d:00 до %02d:00",
				ExpressDelivery, s.Cfg.ShopOpenHour, s.Cfg.ShopCloseHour)
		}
		return nil
	}

	m := deliveryAtRe.FindStringSubmatch(deliveryTime)
	if m == nil {
		return invalid("выберите время доставки")
	}
	h, _ := strconv.Atoi(m[1])
	min, _ := strconv.Atoi(m[2])
	at := h*60 + min
	if at < open || at > closeAt {
		return invalid("доставляем с %02d:00 до %02d:00 — выберите время в этом окне",
			s.Cfg.ShopOpenHour, s.Cfg.ShopCloseHour)
	}
	// На сегодня нужен запас на сборку и дорогу.
	const minLeadMinutes = 60
	if isToday && at < nowMin+minLeadMinutes {
		return invalid("на сегодня принимаем заказы минимум за час — выберите время позже или другую дату")
	}
	return nil
}

func checkLen(field, value string, min, max int) error {
	n := utf8.RuneCountInString(value)
	if n < min {
		return invalid("укажите %s", field)
	}
	if n > max {
		return invalid("%s — не более %d символов", field, max)
	}
	return nil
}

var nonDigits = regexp.MustCompile(`\D`)

// normalizePhone приводит телефон к виду +7XXXXXXXXXX. Курьер должен
// дозвониться — принимаем только то, что похоже на настоящий номер.
func normalizePhone(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	plus := strings.HasPrefix(raw, "+")
	digits := nonDigits.ReplaceAllString(raw, "")

	// Российские номера: 8XXXXXXXXXX и 7XXXXXXXXXX — это один и тот же номер.
	if len(digits) == 11 && (digits[0] == '8' || digits[0] == '7') {
		return "+7" + digits[1:], nil
	}
	if len(digits) == 10 && !plus {
		return "+7" + digits, nil
	}
	if len(digits) < minPhoneDigits || len(digits) > maxPhoneDigits {
		return "", invalid("укажите корректный телефон, например +7 900 000-00-00")
	}
	return "+" + digits, nil
}

// collapseSpaces убирает края и схлопывает подряд идущие пробелы.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// isUniqueViolation — нарушение уникального индекса Postgres (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "23505")
}

// ─── Статусы ───────────────────────────────────────────────────────────────

// TransitionOrder — смена статуса через конечный автомат: только следующий шаг
// либо отмена (с обязательной причиной) из нетерминального статуса.
func (s *Service) TransitionOrder(ctx context.Context, orderID uint, to string, adminID int64, cancelReason string) (*model.Order, error) {
	order, err := s.Repo.GetOrder(ctx, orderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, invalid("заказ не найден")
		}
		return nil, err
	}
	if !model.AllowedTransition(order.Status, to) {
		return nil, invalid("переход «%s» → «%s» недопустим",
			model.StatusLabels[order.Status], model.StatusLabels[to])
	}
	cancelReason = collapseSpaces(cancelReason)
	if to == model.StatusCancelled {
		if cancelReason == "" {
			return nil, invalid("укажите причину отмены")
		}
		if utf8.RuneCountInString(cancelReason) > 200 {
			return nil, invalid("причина отмены — не более 200 символов")
		}
	}
	err = s.Repo.Tx(ctx, func(tx *repository.Repository) error {
		if err := tx.ChangeOrderStatus(ctx, orderID, order.Status, to, adminID, cancelReason); err != nil {
			return err
		}
		// Отменённый заказ обязан вернуть товар на склад — иначе остаток
		// «съедается» навсегда и витрина врёт о наличии.
		if to == model.StatusCancelled {
			return tx.RestoreStockForOrder(ctx, orderID)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, invalid("статус заказа уже изменился — обновите карточку")
		}
		return nil, err
	}
	return s.Repo.GetOrder(ctx, orderID)
}

// ApplyDeepLinkPromo сохраняет промокод у клиента (deep-link ?start=CODE).
func (s *Service) ApplyDeepLinkPromo(ctx context.Context, telegramID int64, code string) (*model.PromoCode, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || utf8.RuneCountInString(code) > maxPromoCodeLen {
		return nil, invalid("промокод не найден")
	}
	promo, err := s.Repo.GetPromoByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, invalid("промокод не найден")
		}
		return nil, err
	}
	if !promo.Usable(s.now()) {
		return nil, invalid("промокод больше не действует")
	}
	user, err := s.Repo.UpsertUser(ctx, telegramID, "", "")
	if err != nil {
		return nil, err
	}
	if err := s.Repo.SetUserPromo(ctx, user.ID, promo.ID); err != nil {
		return nil, err
	}
	return promo, nil
}

// CheckPromo — проверка кода на экране оформления (показать скидку заранее).
func (s *Service) CheckPromo(ctx context.Context, telegramID int64, code string) (*model.PromoCode, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" || utf8.RuneCountInString(code) > maxPromoCodeLen {
		return nil, invalid("промокод не найден")
	}
	promo, err := s.Repo.GetPromoByCode(ctx, code)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, invalid("промокод не найден")
		}
		return nil, err
	}
	if !promo.Usable(s.now()) {
		return nil, invalid("промокод больше не действует")
	}
	// Персональный лимит проверяем сразу: обидно узнать об этом на «подтвердить».
	if promo.PerUserLimit > 0 && telegramID != 0 {
		if user, err := s.Repo.GetUserByTelegramID(ctx, telegramID); err == nil {
			used, err := s.Repo.CountUserRedemptions(ctx, promo.ID, user.ID)
			if err == nil && used >= promo.PerUserLimit {
				return nil, invalid("этот промокод уже использован")
			}
		}
	}
	return promo, nil
}
