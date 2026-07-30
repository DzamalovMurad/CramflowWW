package handler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
)

const promoUsage = "🎟 Промокоды:\n" +
	"/promo new <КОД> <percent|fixed> <значение> [флаги] — создать\n" +
	"/promo list — активные коды со статистикой\n" +
	"/promo info <КОД> — подробная статистика кода\n" +
	"/promo bind <КОД> <telegram_id> — сделать персональным\n" +
	"/promo off <КОД> — отключить\n\n" +
	"Флаги new (все необязательны):\n" +
	"uses=N — общий лимит (0 = без лимита)\n" +
	"peruser=N — лимит на пользователя (по умолчанию 1)\n" +
	"days=N — срок действия в днях\n" +
	"min=РУБ — минимальная сумма заказа\n" +
	"first — только для первого заказа\n" +
	"cat=Категория — только на категорию (Стандарт/Премиум/Люкс/WOW)\n\n" +
	"Примеры:\n" +
	"/promo new SPRING10 percent 10\n" +
	"/promo new GIFT500 fixed 500 uses=50 days=30 min=3000 first"

// Код: 3–20 символов, латиница/кириллица/цифры/дефис/подчёркивание (после UPPERCASE).
var promoCodeRe = regexp.MustCompile(`^[A-ZА-ЯЁ0-9_-]{3,20}$`)

// handlePromo — группа админ-команд /promo (вызывается только для админов).
func (b *Bot) handlePromo(chatID int64, args string) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		b.send(chatID, promoUsage)
		return
	}
	switch fields[0] {
	case "new":
		b.promoNew(chatID, fields[1:])
	case "list":
		b.promoList(chatID)
	case "info":
		if len(fields) < 2 {
			b.send(chatID, "Использование: /promo info <КОД>")
			return
		}
		b.promoInfo(chatID, fields[1])
	case "bind":
		b.promoBind(chatID, fields[1:])
	case "off":
		if len(fields) < 2 {
			b.send(chatID, "Использование: /promo off <КОД>")
			return
		}
		code := strings.ToUpper(fields[1])
		if err := b.repo.SetPromoActive(code, false); err != nil {
			b.send(chatID, "Промокод "+code+" не найден.")
			return
		}
		b.send(chatID, "🚫 Промокод "+code+" отключён.")
	default:
		b.send(chatID, promoUsage)
	}
}

// promoNew — /promo new <КОД> <percent|fixed> <значение> [uses=N] [peruser=N] [days=N] [min=РУБ] [first] [cat=Категория].
func (b *Bot) promoNew(chatID int64, f []string) {
	if len(f) < 3 {
		b.send(chatID, promoUsage)
		return
	}
	code := strings.ToUpper(f[0])
	if !promoCodeRe.MatchString(code) {
		b.send(chatID, "Код — 3–20 символов: буквы, цифры, дефис или подчёркивание.")
		return
	}
	ptype := strings.ToLower(f[1])
	if ptype != model.PromoPercent && ptype != model.PromoFixed {
		b.send(chatID, "Тип скидки — percent (проценты) или fixed (рубли).")
		return
	}
	value, err := strconv.Atoi(f[2])
	if err != nil || value <= 0 || (ptype == model.PromoPercent && value > 99) {
		b.send(chatID, "Значение: для percent — число 1–99, для fixed — сумма в рублях.")
		return
	}

	p := &model.PromoCode{
		Code:           code,
		Type:           ptype,
		Value:          value,
		MaxUsesPerUser: 1,
		IsActive:       true,
		AppliesTo:      model.PromoAppliesAll,
		Origin:         model.PromoOriginManual,
	}
	for _, arg := range f[3:] {
		key, val, _ := strings.Cut(arg, "=")
		n, _ := strconv.Atoi(val)
		switch strings.ToLower(key) {
		case "first":
			p.FirstOrderOnly = true
		case "uses":
			p.MaxUses = n
		case "peruser":
			p.MaxUsesPerUser = n
		case "days":
			if n > 0 {
				exp := time.Now().AddDate(0, 0, n)
				p.ExpiresAt = &exp
			}
		case "min":
			p.MinOrderAmount = n
		case "cat":
			found := false
			for _, c := range model.Categories {
				if strings.EqualFold(c, val) {
					p.AppliesTo = "category:" + c
					found = true
					break
				}
			}
			if !found {
				b.send(chatID, "Неизвестная категория «"+val+"». Доступны: "+strings.Join(model.Categories, ", "))
				return
			}
		default:
			b.send(chatID, "Неизвестный флаг «"+arg+"».\n\n"+promoUsage)
			return
		}
	}
	if err := b.repo.CreatePromo(p); err != nil {
		b.send(chatID, "Не удалось создать (возможно, код "+code+" уже существует): "+err.Error())
		return
	}
	b.send(chatID, "✅ Промокод создан.\n\n"+formatPromo(p, nil))
}

// promoBind — /promo bind <КОД> <telegram_id>: код становится персональным.
func (b *Bot) promoBind(chatID int64, f []string) {
	if len(f) < 2 {
		b.send(chatID, "Использование: /promo bind <КОД> <telegram_id>")
		return
	}
	promo, err := b.repo.GetPromoByCode(f[0])
	if err != nil {
		b.send(chatID, "Промокод "+strings.ToUpper(f[0])+" не найден.")
		return
	}
	tgID, err := strconv.ParseInt(f[1], 10, 64)
	if err != nil || tgID == 0 {
		b.send(chatID, "Некорректный telegram_id.")
		return
	}
	user, err := b.repo.UpsertUser(tgID, "", "")
	if err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	if err := b.repo.BindPromoToUser(promo.ID, user.ID); err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	who := orDash(user.Name)
	b.send(chatID, fmt.Sprintf("🔒 Промокод %s теперь персональный: только для %s (telegram_id %d).",
		promo.Code, who, tgID))
}

// promoList — /promo list: активные коды со статистикой одной строкой на код.
func (b *Bot) promoList(chatID int64) {
	promos, err := b.repo.ListPromos(50)
	if err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	stats, err := b.repo.GetPromoStatsMap()
	if err != nil {
		stats = map[uint]repository.PromoStats{}
	}
	var sb strings.Builder
	n := 0
	for _, p := range promos {
		if !p.IsActive {
			continue
		}
		n++
		st := stats[p.ID]
		fmt.Fprintf(&sb, "• %s — скидка %s", p.Code, p.DiscountLabel())
		if p.MaxUses > 0 {
			fmt.Fprintf(&sb, " · %d/%d", p.UsedCount, p.MaxUses)
		} else {
			fmt.Fprintf(&sb, " · исп. %d", p.UsedCount)
		}
		fmt.Fprintf(&sb, " · скидок %d₽ · выручка %d₽", st.TotalDiscount, st.Revenue)
		if p.ExpiresAt != nil {
			fmt.Fprintf(&sb, " · до %s", p.ExpiresAt.In(service.MskLocation).Format("02.01"))
		}
		sb.WriteString("\n")
	}
	if n == 0 {
		b.send(chatID, "Активных промокодов нет. Создать: /promo new")
		return
	}
	b.send(chatID, fmt.Sprintf("🎟 Активные промокоды (%d):\n\n%s\nПодробнее: /promo info <КОД>", n, sb.String()))
}

// promoInfo — /promo info <КОД>: полная карточка кода со статистикой.
func (b *Bot) promoInfo(chatID int64, code string) {
	promo, err := b.repo.GetPromoByCode(code)
	if err != nil {
		b.send(chatID, "Промокод "+strings.ToUpper(code)+" не найден.")
		return
	}
	stats, err := b.repo.GetPromoStatsMap()
	if err != nil {
		stats = map[uint]repository.PromoStats{}
	}
	st := stats[promo.ID]
	b.send(chatID, formatPromo(promo, &st))
}

// formatPromo — карточка промокода для админ-чата (stats == nil — без статистики).
func formatPromo(p *model.PromoCode, stats *repository.PromoStats) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎟 %s — скидка %s\n", p.Code, p.DiscountLabel())
	if p.IsActive {
		sb.WriteString("Статус: ✅ активен\n")
	} else {
		sb.WriteString("Статус: 🚫 отключён\n")
	}
	if p.MaxUses > 0 {
		fmt.Fprintf(&sb, "Использований: %d/%d\n", p.UsedCount, p.MaxUses)
	} else {
		fmt.Fprintf(&sb, "Использований: %d (без лимита)\n", p.UsedCount)
	}
	if p.MaxUsesPerUser > 0 {
		fmt.Fprintf(&sb, "На пользователя: %d\n", p.MaxUsesPerUser)
	}
	if p.StartsAt != nil {
		fmt.Fprintf(&sb, "Действует с %s\n", p.StartsAt.In(service.MskLocation).Format("02.01.2006"))
	}
	if p.ExpiresAt != nil {
		fmt.Fprintf(&sb, "Действует до %s\n", p.ExpiresAt.In(service.MskLocation).Format("02.01.2006"))
	} else {
		sb.WriteString("Срок: бессрочный\n")
	}
	if p.MinOrderAmount > 0 {
		fmt.Fprintf(&sb, "Мин. сумма заказа: %d₽\n", p.MinOrderAmount)
	}
	switch {
	case strings.HasPrefix(p.AppliesTo, "category:"):
		fmt.Fprintf(&sb, "Область: категория «%s»\n", strings.TrimPrefix(p.AppliesTo, "category:"))
	case strings.HasPrefix(p.AppliesTo, "products:"):
		fmt.Fprintf(&sb, "Область: товары #%s\n", strings.TrimPrefix(p.AppliesTo, "products:"))
	}
	if p.FirstOrderOnly {
		sb.WriteString("Только для первого заказа\n")
	}
	if p.BoundUserID != nil {
		sb.WriteString("🔒 Персональный\n")
	}
	if p.Origin != "" && p.Origin != model.PromoOriginManual {
		fmt.Fprintf(&sb, "Происхождение: %s\n", p.Origin)
	}
	if stats != nil {
		fmt.Fprintf(&sb, "\nВыкуплено: %d · Скидок выдано: %d₽ · Выручка с заказов: %d₽",
			stats.Redemptions, stats.TotalDiscount, stats.Revenue)
	}
	return strings.TrimSpace(sb.String())
}
