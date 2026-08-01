package handler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/service"
)

// Промокоды в чате админа: список с переключателем и визард создания.
// Раньше акцию можно было завести только руками в SQL — то есть практически
// никогда.

// ─── Список ────────────────────────────────────────────────────────────────

func (b *Bot) sendPromoList(ctx context.Context, chatID int64) {
	promos, err := b.svc.ListPromos(ctx)
	if err != nil {
		b.adminError(b.log, chatID, "list promos", err)
		return
	}
	if len(promos) == 0 {
		b.send(chatID, "Промокодов пока нет. Создать: /promoadd")
		return
	}

	var sb strings.Builder
	sb.WriteString("Промокоды:\n\n")
	var rows [][]tgbotapi.InlineKeyboardButton
	for i := range promos {
		p := &promos[i]
		sb.WriteString(promoSummary(p, b.cfg.Now()))
		sb.WriteString("\n")

		label, action := "Выключить "+p.Code, "off"
		if !p.IsActive {
			label, action = "Включить "+p.Code, "on"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(clipLabel(label), fmt.Sprintf("pr:%s:%d", action, p.ID))))
		// Клавиатура Telegram на сотню кнопок не рассчитана: переключаем
		// свежие акции, остальные видно текстом.
		if len(rows) >= 20 {
			break
		}
	}
	rows = append(rows, closeRow())
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendKb(chatID, clipTelegram(sb.String()), &kb)
}

// promoSummary — одна строка описания акции для чата.
func promoSummary(p *model.PromoCode, now time.Time) string {
	var sb strings.Builder
	mark := "🟢"
	if !p.Usable(now) {
		mark = "⚪️"
	}
	fmt.Fprintf(&sb, "%s %s — %s", mark, p.Code, p.Describe())
	if p.MinOrderAmount > 0 {
		fmt.Fprintf(&sb, ", от %d₽", p.MinOrderAmount)
	}
	if p.MaxUses > 0 {
		fmt.Fprintf(&sb, ", использован %d/%d", p.Uses, p.MaxUses)
	} else {
		fmt.Fprintf(&sb, ", использован %d раз", p.Uses)
	}
	if p.PerUserLimit > 0 {
		fmt.Fprintf(&sb, ", по %d на клиента", p.PerUserLimit)
	}
	if p.ExpiresAt != nil {
		fmt.Fprintf(&sb, ", до %s", p.ExpiresAt.In(now.Location()).Format("02.01.2006"))
	}
	if !p.IsActive {
		sb.WriteString(" (выключен)")
	}
	return sb.String()
}

// promoMinimumHint — приписка про минимальную сумму для сообщений клиенту.
func promoMinimumHint(p *model.PromoCode) string {
	if p.MinOrderAmount <= 0 {
		return ""
	}
	return fmt.Sprintf(" (на заказ от %d₽)", p.MinOrderAmount)
}

// orderPromoCode — код, применённый в заказе: сначала снимок в самом заказе,
// потом связанная акция (у старых заказов снимка нет).
func orderPromoCode(o *model.Order) string {
	if o.AppliedPromoCode != "" {
		return o.AppliedPromoCode
	}
	if o.PromoCode != nil {
		return o.PromoCode.Code
	}
	return ""
}

// ─── Callback переключателя ────────────────────────────────────────────────

func (b *Bot) handlePromoCallback(ctx context.Context, log *slog.Logger, cb *tgbotapi.CallbackQuery, parts []string, argAt func(int) uint) {
	chatID := cb.Message.Chat.ID
	if len(parts) < 3 {
		return
	}
	active := parts[1] == "on"
	promo, err := b.svc.SetPromoActive(ctx, argAt(2), active)
	if err != nil {
		b.adminError(log, chatID, "toggle promo", err)
		return
	}
	state := "выключен"
	if promo.IsActive {
		state = "включён"
	}
	b.sendTemp(chatID, fmt.Sprintf("Промокод %s %s.", promo.Code, state), 5*time.Second)
	// Перерисовываем список, чтобы кнопка совпадала с реальным состоянием.
	b.sendPromoList(ctx, chatID)
}

// ─── Визард создания ───────────────────────────────────────────────────────

// Шаги: код → скидка → минимальная сумма → срок → лимиты.
const (
	promoStepCode     = "code"
	promoStepDiscount = "discount"
	promoStepMin      = "min"
	promoStepExpires  = "expires"
	promoStepLimits   = "limits"
)

func (b *Bot) startPromoWizard(chatID int64) {
	b.setWizard(chatID, &wizard{mode: "promo_add", step: promoStepCode})
	b.send(chatID, "🎁 Новый промокод.\n\n"+
		"Шаг 1/5 — код латиницей, например WELCOME10.\n"+
		"Клиент вводит его при оформлении или получает ссылкой.")
}

// promoWizardInput — один шаг визарда. Возвращать ошибки клиенту здесь важнее,
// чем экономить сообщения: админ вводит акцию с телефона и вслепую.
func (b *Bot) promoWizardInput(ctx context.Context, log *slog.Logger, chatID int64, w *wizard, text string) {
	switch w.step {
	case promoStepCode:
		code := strings.ToUpper(strings.TrimSpace(text))
		if err := service.ValidatePromoCode(code); err != nil {
			b.adminError(log, chatID, "promo code", err)
			return
		}
		// Занятый код ловим сразу: обиднее узнать об этом после пяти шагов.
		if _, err := b.repo.GetPromoByCode(ctx, code); err == nil {
			b.send(chatID, fmt.Sprintf("Промокод %s уже существует. Введите другой код.", code))
			return
		}
		w.promo.code = code
		w.step = promoStepDiscount
		b.send(chatID, "Шаг 2/5 — размер скидки:\n"+
			"• «15%» — процент от суммы заказа;\n"+
			"• «500» — фиксированная скидка в рублях.")

	case promoStepDiscount:
		typ, value, err := parsePromoDiscount(text)
		if err != nil {
			b.send(chatID, capitalize(err.Error())+". Например: 15% или 500")
			return
		}
		if err := service.ValidatePromoRule(typ, value, 0); err != nil {
			b.adminError(log, chatID, "promo rule", err)
			return
		}
		w.promo.discountType, w.promo.discountValue = typ, value
		w.step = promoStepMin
		b.send(chatID, "Шаг 3/5 — минимальная сумма заказа в рублях.\n«-» — без ограничения.")

	case promoStepMin:
		amount, err := parseOptionalAmount(text)
		if err != nil {
			b.send(chatID, capitalize(err.Error())+". Например: 3000 или -")
			return
		}
		// Порог проверяем вместе со скидкой: «−1000₽ от 1000₽» — бесплатный заказ.
		if err := service.ValidatePromoRule(w.promo.discountType, w.promo.discountValue, amount); err != nil {
			b.adminError(log, chatID, "promo rule", err)
			return
		}
		w.promo.minOrder = amount
		w.step = promoStepExpires
		b.send(chatID, "Шаг 4/5 — до какого числа действует: ДД.ММ.ГГГГ.\n«-» — бессрочно.")

	case promoStepExpires:
		expires, err := b.parsePromoExpiry(text)
		if err != nil {
			b.send(chatID, capitalize(err.Error())+". Например: 31.12.2026 или -")
			return
		}
		if expires != nil && !expires.After(b.cfg.Now()) {
			b.send(chatID, "Эта дата уже прошла. Укажите будущую или «-» — бессрочно.")
			return
		}
		w.promo.expiresAt = expires
		w.step = promoStepLimits
		b.send(chatID, "Шаг 5/5 — лимиты: «всего на клиента», например «100 1».\n"+
			"«-» — без ограничений.\nПо умолчанию клиент может применить код один раз.")

	case promoStepLimits:
		maxUses, perUser, err := parsePromoLimits(text)
		if err != nil {
			b.send(chatID, capitalize(err.Error())+". Например: 100 1")
			return
		}
		b.finishPromoWizard(ctx, log, chatID, w, maxUses, perUser)
	}
}

func (b *Bot) finishPromoWizard(ctx context.Context, log *slog.Logger, chatID int64, w *wizard, maxUses, perUser int) {
	promo, err := b.svc.CreatePromo(ctx, service.PromoInput{
		Code:           w.promo.code,
		DiscountType:   w.promo.discountType,
		DiscountValue:  w.promo.discountValue,
		MinOrderAmount: w.promo.minOrder,
		MaxUses:        maxUses,
		PerUserLimit:   perUser,
		ExpiresAt:      w.promo.expiresAt,
	})
	if err != nil {
		// Код, скидку и порог проверили на своих шагах, поэтому сюда доходят
		// только неисправимые на месте случаи (например, код заняли параллельно).
		b.clearWizard(chatID)
		b.adminError(log, chatID, "create promo", err)
		b.send(chatID, "Промокод не создан. Начните заново: /promoadd")
		return
	}
	b.clearWizard(chatID)

	msg := "✅ Промокод создан:\n" + promoSummary(promo, b.cfg.Now())
	if link := b.promoDeepLink(promo.Code); link != "" {
		msg += "\n\nСсылка для клиента:\n" + link
	}
	if promo.DiscountType == model.DiscountTypeFixed {
		msg += "\n\n⚠️ Фиксированная скидка применяется при оформлении, " +
			"но в корзине Mini App пока не показывается предварительно."
	}
	b.send(chatID, msg)
}

// promoDeepLink — ссылка, которая сразу закрепляет код за клиентом.
func (b *Bot) promoDeepLink(code string) string {
	if b.Username() == "" {
		return ""
	}
	return fmt.Sprintf("https://t.me/%s?start=%s", b.Username(), code)
}

// ─── Разбор ввода ──────────────────────────────────────────────────────────

// parsePromoDiscount понимает «15%» как процент и «500» как рубли.
func parsePromoDiscount(s string) (typ string, value int, err error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, " ", ""))
	s = strings.TrimSuffix(strings.TrimSuffix(s, "₽"), "р")
	typ = model.DiscountTypeFixed
	if strings.HasSuffix(s, "%") {
		typ = model.DiscountTypePercent
		s = strings.TrimSuffix(s, "%")
	}
	value, convErr := strconv.Atoi(s)
	if convErr != nil {
		return "", 0, fmt.Errorf("не понял размер скидки")
	}
	if value < 1 {
		return "", 0, fmt.Errorf("скидка должна быть больше нуля")
	}
	return typ, value, nil
}

// parseOptionalAmount — сумма в рублях или «-» (ноль = без ограничения).
func parseOptionalAmount(s string) (int, error) {
	s = strings.TrimSpace(s)
	if isSkip(s) {
		return 0, nil
	}
	v, err := strconv.Atoi(strings.ReplaceAll(s, " ", ""))
	if err != nil || v < 0 {
		return 0, fmt.Errorf("не понял сумму")
	}
	return v, nil
}

// parsePromoLimits разбирает «100 1»: всего применений и сколько на клиента.
// Одно число — общий лимит, «-» — без ограничений, но один раз на клиента:
// безлимитный на клиента код по умолчанию — это подарок фродерам.
func parsePromoLimits(s string) (maxUses, perUser int, err error) {
	s = strings.TrimSpace(s)
	if isSkip(s) {
		return 0, 1, nil
	}
	fields := strings.Fields(s)
	if len(fields) > 2 {
		return 0, 0, fmt.Errorf("нужно одно или два числа")
	}
	perUser = 1
	for i, f := range fields {
		v, convErr := strconv.Atoi(f)
		if convErr != nil || v < 0 {
			return 0, 0, fmt.Errorf("не понял число «%s»", f)
		}
		if i == 0 {
			maxUses = v
		} else {
			perUser = v
		}
	}
	return maxUses, perUser, nil
}

// parsePromoExpiry — дата ДД.ММ.ГГГГ в часовом поясе магазина, до конца суток.
func (b *Bot) parsePromoExpiry(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if isSkip(s) {
		return nil, nil
	}
	d, err := time.ParseInLocation("02.01.2006", s, b.cfg.Location)
	if err != nil {
		// Заодно принимаем формат, в котором даты приходят из API.
		if d, err = time.ParseInLocation("2006-01-02", s, b.cfg.Location); err != nil {
			return nil, fmt.Errorf("не понял дату")
		}
	}
	// «до 31.12» для админа означает «включая 31 декабря».
	end := d.Add(24*time.Hour - time.Second)
	return &end, nil
}

func isSkip(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "-", "—", "–", "нет", "no", "0":
		return true
	}
	return false
}
