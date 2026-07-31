package handler

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
)

// FALLBACK-РЕЖИМ ЗАКАЗА.
//
// Если Mini App недоступен (упал фронтенд, не задан TELEGRAM_APP_URL, у клиента
// старый Telegram), заказ всё равно должен оформляться — обычным диалогом в чате.
// Красоты тут нет и не будет: список кнопок, количество, адрес, телефон.
// Заказ создаётся тем же service.CreateOrder, что и из Mini App, поэтому в CRM,
// уведомлениях и статусах он ничем не отличается от обычного.
//
// Включается командой администратора /fallback on. Когда TELEGRAM_APP_URL пуст,
// режим считается включённым принудительно: другого способа заказать нет.

// fallbackOrder — состояние диалога заказа у конкретного клиента.
type fallbackOrder struct {
	step        string // product → variant → qty → address → phone → confirm
	productID   uint
	productName string
	variantID   uint
	flowers     int // цветов в букете (из варианта)
	price       int
	qty         int
	address     string
	phone       string
	name        string
	startedAt   time.Time
}

// fallbackTTL — брошенный диалог не висит вечно: через час начинаем заново,
// иначе клиент, вернувшийся назавтра, ответит «да» на давно забытый вопрос.
const fallbackTTL = time.Hour

// fallbackState — диалоги клиентов (в отличие от админских визардов их много
// и они идут параллельно, поэтому отдельная карта под мьютексом).
type fallbackState struct {
	mu     sync.Mutex
	orders map[int64]*fallbackOrder
}

func newFallbackState() *fallbackState {
	return &fallbackState{orders: map[int64]*fallbackOrder{}}
}

func (s *fallbackState) get(chatID int64) (*fallbackOrder, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.orders[chatID]
	if ok && time.Since(o.startedAt) > fallbackTTL {
		delete(s.orders, chatID)
		return nil, false
	}
	return o, ok
}

func (s *fallbackState) set(chatID int64, o *fallbackOrder) {
	o.startedAt = time.Now()
	s.mu.Lock()
	s.orders[chatID] = o
	s.mu.Unlock()
}

func (s *fallbackState) clear(chatID int64) {
	s.mu.Lock()
	delete(s.orders, chatID)
	s.mu.Unlock()
}

// fallbackOrdersOn — единое правило доступности заказа через диалог, общее
// для бота и для /api/config. Без адреса Mini App режим включён всегда:
// иначе магазин не продаёт вообще ничем.
//
// Правило живёт в одном месте намеренно: если бот считает режим включённым,
// а фронтенд — выключенным, клиент получит два разных ответа на один вопрос.
func fallbackOrdersOn(repo *repository.Repository, appURL string) bool {
	return appURL == "" || repo.FallbackOrdersEnabled()
}

// fallbackEnabled — доступен ли заказ через диалог бота.
func (b *Bot) fallbackEnabled() bool {
	return fallbackOrdersOn(b.repo, b.appURL)
}

// setFallback — админская команда /fallback on|off.
func (b *Bot) setFallback(chatID int64, arg string) {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on":
		if err := b.repo.SetFallbackOrders(true); err != nil {
			b.send(chatID, "Ошибка: "+err.Error())
			return
		}
		b.send(chatID, "✅ Заказ через диалог бота включён.\nКлиенты увидят кнопку «Заказать в чате» и команду /order.")
	case "off":
		if err := b.repo.SetFallbackOrders(false); err != nil {
			b.send(chatID, "Ошибка: "+err.Error())
			return
		}
		if b.appURL == "" {
			b.send(chatID, "⚠️ Настройка сохранена, но TELEGRAM_APP_URL не задан — "+
				"диалог остаётся единственным способом заказать и продолжит работать.")
			return
		}
		b.send(chatID, "✅ Заказ через диалог выключен — клиенты оформляют заказ в Mini App.")
	default:
		state := "выключен"
		if b.fallbackEnabled() {
			state = "включён"
		}
		b.send(chatID, fmt.Sprintf("Сейчас режим заказа через диалог %s.\n\nИспользование: /fallback on | /fallback off", state))
	}
}

// --- Диалог клиента ---

// handleFallbackCommand — /order: старт диалога со списка товаров.
func (b *Bot) handleFallbackCommand(chatID int64) {
	if !b.fallbackEnabled() {
		b.sendShopButton(chatID, "Заказы принимаем в приложении — откройте магазин кнопкой ниже 🌸")
		return
	}
	products, err := b.repo.TopHits(10)
	if err != nil || len(products) == 0 {
		b.send(chatID, "Каталог сейчас пуст. Загляните чуть позже 🌸")
		return
	}

	var sb strings.Builder
	sb.WriteString("🌸 Заказ в чате.\n\nВыберите букет:\n\n")
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range products {
		if len(p.Variants) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "• %s — от %d₽\n", p.Name, p.Variants[0].Price)
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				trimButton(fmt.Sprintf("%s · от %d₽", p.Name, p.Variants[0].Price)),
				fmt.Sprintf("fo:%d", p.ID))))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Отмена", "fcancel")))

	b.fallback.set(chatID, &fallbackOrder{step: "product"})
	b.sendKb(chatID, sb.String(), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// trimButton — Telegram обрезает длинные подписи кнопок сам, но криво.
func trimButton(s string) string {
	r := []rune(s)
	if len(r) <= 40 {
		return s
	}
	return string(r[:39]) + "…"
}

// isFallbackCallback — принадлежит ли кнопка клиентскому диалогу заказа.
// Список явный: у админских кнопок есть свои «f»-префиксы (flt, flt2).
func isFallbackCallback(data string) bool {
	switch data {
	case "forder", "fcancel", "fok":
		return true
	}
	return strings.HasPrefix(data, "fo:") || strings.HasPrefix(data, "fv:")
}

// handleFallbackCallback — нажатия кнопок в диалоге заказа (не админские).
// Возвращает false, если callback не наш.
func (b *Bot) handleFallbackCallback(cb *tgbotapi.CallbackQuery) bool {
	data := cb.Data
	chatID := cb.Message.Chat.ID

	switch {
	case data == "fcancel":
		b.fallback.clear(chatID)
		b.send(chatID, "Заказ отменён. Начать заново — /order")
		return true

	case data == "forder":
		b.handleFallbackCommand(chatID)
		return true

	case strings.HasPrefix(data, "fo:"): // выбран товар → показываем размеры
		id, _ := strconv.ParseUint(strings.TrimPrefix(data, "fo:"), 10, 32)
		p, err := b.repo.GetProduct(uint(id))
		if err != nil || p.IsHidden || len(p.Variants) == 0 {
			b.send(chatID, "Этот букет уже недоступен. Выберите другой: /order")
			return true
		}
		var rows [][]tgbotapi.InlineKeyboardButton
		for _, v := range p.Variants {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(
					fmt.Sprintf("%d шт — %d₽", v.Quantity, v.Price),
					fmt.Sprintf("fv:%d", v.ID))))
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("← К списку", "forder"),
			tgbotapi.NewInlineKeyboardButtonData("✖️ Отмена", "fcancel")))

		b.fallback.set(chatID, &fallbackOrder{step: "variant", productID: p.ID, productName: p.Name})
		b.sendKb(chatID, fmt.Sprintf("«%s»\n\nВыберите размер букета:", p.Name),
			tgbotapi.NewInlineKeyboardMarkup(rows...))
		return true

	case strings.HasPrefix(data, "fv:"): // выбран размер → спрашиваем количество
		id, _ := strconv.ParseUint(strings.TrimPrefix(data, "fv:"), 10, 32)
		v, err := b.repo.GetVariant(uint(id))
		if err != nil {
			b.send(chatID, "Этот размер уже недоступен. Выберите другой: /order")
			return true
		}
		o, ok := b.fallback.get(chatID)
		if !ok {
			o = &fallbackOrder{}
		}
		p, err := b.repo.GetProduct(v.ProductID)
		if err != nil || p.IsHidden {
			b.send(chatID, "Этот букет уже недоступен. Выберите другой: /order")
			return true
		}
		o.step = "qty"
		o.productID = p.ID
		o.productName = p.Name
		o.variantID = v.ID
		o.flowers = v.Quantity
		o.price = v.Price
		b.fallback.set(chatID, o)

		b.askReply(chatID, fmt.Sprintf(
			"«%s», %d шт в букете — %d₽.\n\nСколько таких букетов нужно? Пришлите число (1–99).",
			p.Name, v.Quantity, v.Price), "например: 1")
		return true
	}
	return false
}

// askReply задаёт вопрос с ForceReply: у клиента сразу открывается поле ввода
// с цитированием вопроса — в чате не потеряться, на каком мы шаге.
func (b *Bot) askReply(chatID int64, text, placeholder string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, InputFieldPlaceholder: placeholder}
	if _, err := b.tgSend(msg); err != nil {
		log.Printf("bot send: %v", err)
	}
}

// handleFallbackInput — текстовые ответы клиента в диалоге заказа.
// Возвращает false, если диалог не идёт и сообщение нужно обработать иначе.
func (b *Bot) handleFallbackInput(msg *tgbotapi.Message) bool {
	chatID := msg.Chat.ID
	o, ok := b.fallback.get(chatID)
	if !ok {
		return false
	}
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return false
	}

	switch o.step {
	case "qty":
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > 99 {
			b.askReply(chatID, "Нужно число от 1 до 99. Сколько букетов?", "например: 1")
			return true
		}
		o.qty = n
		o.step = "address"
		b.fallback.set(chatID, o)
		b.askReply(chatID, "Куда доставить? Напишите адрес: улица, дом, квартира.",
			"например: Тверская 12, кв. 5")
		return true

	case "address":
		if len([]rune(text)) < 5 {
			b.askReply(chatID, "Адрес слишком короткий. Напишите улицу, дом и квартиру.",
				"например: Тверская 12, кв. 5")
			return true
		}
		o.address = text
		o.step = "phone"
		b.fallback.set(chatID, o)
		b.askReply(chatID, "Ваш телефон для связи с курьером?", "+7 900 000-00-00")
		return true

	case "phone":
		if len(digitsOnly(text)) < 6 {
			b.askReply(chatID, "Не похоже на телефон. Пришлите номер целиком.", "+7 900 000-00-00")
			return true
		}
		o.phone = text
		o.name = strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
		if o.name == "" {
			o.name = "Клиент из бота"
		}
		o.step = "confirm"
		b.fallback.set(chatID, o)

		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Оформить", "fok"),
			tgbotapi.NewInlineKeyboardButtonData("✖️ Отмена", "fcancel"),
		))
		b.sendKb(chatID, fmt.Sprintf(
			"Проверьте заказ:\n\n🌸 %s, %d шт в букете\nКоличество: %d\nИтого: %d₽\n\n"+
				"Имя: %s\nТелефон: %s\nАдрес: %s\nДоставка: сегодня, в течение часа",
			o.productName, o.flowers, o.qty, o.price*o.qty, o.name, o.phone, o.address), kb)
		return true
	}
	return false
}

// confirmFallbackOrder — кнопка «Оформить»: создаём заказ тем же путём,
// что и Mini App (service.CreateOrder), включая уведомление админам.
func (b *Bot) confirmFallbackOrder(chatID int64, telegramID int64) {
	o, ok := b.fallback.get(chatID)
	if !ok || o.step != "confirm" {
		b.send(chatID, "Заказ уже неактуален. Начните заново: /order")
		return
	}
	b.fallback.clear(chatID)

	order, err := b.svc.CreateOrder(service.OrderInput{
		Items:           []service.OrderItemInput{{VariantID: o.variantID, Quantity: o.qty}},
		Name:            o.name,
		Phone:           o.phone,
		DeliveryAddress: o.address,
		DeliveryDate:    time.Now().Format("2006-01-02"),
		DeliveryTime:    service.DeliveryOptions[0], // «в течение часа»
		Comment:         "Заказ оформлен через диалог бота (fallback-режим)",
		TelegramID:      telegramID,
	})
	if err != nil {
		b.send(chatID, "Не получилось оформить заказ: "+err.Error()+"\n\nПопробуйте ещё раз: /order")
		return
	}
	b.send(chatID, fmt.Sprintf(
		"✅ Заказ #%d принят!\n\n🌸 %s ×%d — %d₽\nАдрес: %s\n\n"+
			"Менеджер позвонит вам для подтверждения. Спасибо! 🌸",
		order.ID, o.productName, o.qty, order.TotalPrice, o.address))
}

func digitsOnly(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// fallbackHint — что предложить клиенту, когда Mini App недоступен.
func (b *Bot) fallbackHint() (string, tgbotapi.InlineKeyboardMarkup) {
	return "🌸 Добро пожаловать в Flowix!\n\nПриложение магазина сейчас недоступно, " +
			"но заказ можно оформить прямо здесь, в чате.",
		tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🌸 Заказать в чате", "forder")))
}
