package handler

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

type Bot struct {
	api         *tgbotapi.BotAPI
	repo        *repository.Repository
	svc         *service.Service
	store       storage.Storage
	adminChatID int64
	appURL      string

	// Состояние визардов /add и /edit. Админ один, поэтому простая map без мьютекса:
	// все апдейты обрабатываются последовательно в Run().
	wizards map[int64]*wizard
}

type wizard struct {
	mode      string // add | edit_text | edit_variants | edit_photos
	step      string // для add: name → photos → desc → variants → category → confirm
	productID uint   // для edit
	field     string // name | description
	draft     draft
}

type draft struct {
	name        string
	description string
	category    string
	variants    []model.ProductVariant
	imageURLs   []string
}

func NewBot(token string, adminChatID int64, appURL string, repo *repository.Repository, svc *service.Service, store storage.Storage) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	b := &Bot{
		api:         api,
		repo:        repo,
		svc:         svc,
		store:       store,
		adminChatID: adminChatID,
		appURL:      appURL,
		wizards:     map[int64]*wizard{},
	}
	svc.NotifyNewOrder = b.NotifyNewOrder
	return b, nil
}

func (b *Bot) Run() {
	log.Printf("бот запущен: @%s", b.api.Self.UserName)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	for update := range b.api.GetUpdatesChan(u) {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("bot panic: %v", r)
				}
			}()
			switch {
			case update.CallbackQuery != nil:
				b.handleCallback(update.CallbackQuery)
			case update.Message != nil:
				b.handleMessage(update.Message)
			}
		}()
	}
}

func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("bot send: %v", err)
	}
}

func (b *Bot) sendKb(chatID int64, text string, kb tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = kb
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("bot send: %v", err)
	}
}

// --- Входящие сообщения ---

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	if msg.Chat.ID != b.adminChatID {
		b.handleCustomer(msg)
		return
	}

	if msg.IsCommand() {
		switch msg.Command() {
		case "start", "help":
			b.send(msg.Chat.ID, "Команды администратора:\n"+
				"/add — добавить товар\n"+
				"/edit — изменить товар\n"+
				"/hide — скрыть/показать товар\n"+
				"/delete — удалить товар\n"+
				"/orders — последние заказы\n"+
				"/cancel — прервать текущее действие")
		case "add":
			b.wizards[msg.Chat.ID] = &wizard{mode: "add", step: "name"}
			b.send(msg.Chat.ID, "🌸 Новый товар.\n\nШаг 1/5 — введите название:")
		case "edit":
			b.sendProductList(msg.Chat.ID, "Что редактируем?", "edit")
		case "hide":
			b.sendProductList(msg.Chat.ID, "Какой товар скрыть/показать?", "hide")
		case "delete":
			b.sendProductList(msg.Chat.ID, "Какой товар удалить?", "del")
		case "orders":
			b.sendOrderList(msg.Chat.ID)
		case "done":
			b.wizardDone(msg.Chat.ID)
		case "cancel":
			delete(b.wizards, msg.Chat.ID)
			b.send(msg.Chat.ID, "Действие отменено.")
		default:
			b.send(msg.Chat.ID, "Неизвестная команда. /help — список команд.")
		}
		return
	}

	if w, ok := b.wizards[msg.Chat.ID]; ok {
		b.wizardInput(msg, w)
		return
	}
	b.send(msg.Chat.ID, "Используйте команды: /add /edit /hide /delete /orders")
}

// handleCustomer — не-админ: deep-link промокоды и кнопка Mini App.
func (b *Bot) handleCustomer(msg *tgbotapi.Message) {
	if msg.IsCommand() && msg.Command() == "start" {
		if code := strings.TrimSpace(msg.CommandArguments()); code != "" {
			promo, err := b.svc.ApplyDeepLinkPromo(msg.From.ID, code)
			if err == nil {
				b.sendShopButton(msg.Chat.ID, fmt.Sprintf(
					"🎁 Промокод %s активирован — скидка %d%%!\nОн применится автоматически при оформлении заказа.",
					promo.Code, promo.DiscountPercent))
				return
			}
			b.sendShopButton(msg.Chat.ID, "К сожалению, такой промокод не найден. Но цветы всё равно ждут вас 🌸")
			return
		}
	}
	b.sendShopButton(msg.Chat.ID, "🌸 Добро пожаловать в CramFlow!\nВыбирайте букеты в нашем магазине:")
}

// webAppKeyboard — inline-кнопка с web_app (запуск Mini App). В tgbotapi v5.5.1
// такого поля нет, поэтому собираем JSON-совместимую структуру сами:
// библиотека сериализует ReplyMarkup через json.Marshal как есть.
type webAppKeyboard struct {
	InlineKeyboard [][]webAppButton `json:"inline_keyboard"`
}

type webAppButton struct {
	Text   string `json:"text"`
	WebApp struct {
		URL string `json:"url"`
	} `json:"web_app"`
}

func (b *Bot) sendShopButton(chatID int64, text string) {
	if b.appURL == "" {
		b.send(chatID, text)
		return
	}
	btn := webAppButton{Text: "🌸 Открыть магазин"}
	btn.WebApp.URL = b.appURL
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = webAppKeyboard{InlineKeyboard: [][]webAppButton{{btn}}}
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("bot send: %v", err)
	}
}

// --- Визард: ввод текста и фото ---

var variantRe = regexp.MustCompile(`(\d+)\s*шт\s*[—–-]+\s*(\d+)\s*₽?`)

func parseVariants(s string) []model.ProductVariant {
	var out []model.ProductVariant
	for _, m := range variantRe.FindAllStringSubmatch(s, -1) {
		qty, _ := strconv.Atoi(m[1])
		price, _ := strconv.Atoi(m[2])
		if qty > 0 && price > 0 {
			out = append(out, model.ProductVariant{Quantity: qty, Price: price})
		}
	}
	return out
}

func (b *Bot) wizardInput(msg *tgbotapi.Message, w *wizard) {
	chatID := msg.Chat.ID

	// Приём фото (шаг photos в /add или режим замены фото в /edit).
	if len(msg.Photo) > 0 {
		if (w.mode == "add" && w.step == "photos") || w.mode == "edit_photos" {
			url, err := b.savePhoto(msg.Photo)
			if err != nil {
				log.Printf("save photo: %v", err)
				b.send(chatID, "Не удалось сохранить фото, попробуйте ещё раз.")
				return
			}
			w.draft.imageURLs = append(w.draft.imageURLs, url)
			n := len(w.draft.imageURLs)
			if n >= 5 {
				b.wizardDone(chatID)
				return
			}
			b.send(chatID, fmt.Sprintf("📷 Фото %d/5 сохранено. Отправьте ещё или /done, чтобы продолжить.", n))
			return
		}
		b.send(chatID, "Фото сейчас не ожидается.")
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	switch w.mode {
	case "add":
		b.wizardAddText(chatID, w, text)
	case "edit_text":
		p, err := b.repo.GetProduct(w.productID)
		if err != nil {
			delete(b.wizards, chatID)
			b.send(chatID, "Товар не найден.")
			return
		}
		if w.field == "name" {
			p.Name = text
		} else {
			if text == "-" {
				text = ""
			}
			p.Description = text
		}
		if err := b.repo.SaveProduct(p); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		delete(b.wizards, chatID)
		b.send(chatID, "✅ Изменения сохранены.")
	case "edit_variants":
		variants := parseVariants(text)
		if len(variants) == 0 {
			b.send(chatID, "Не понял формат. Пример: 9 шт — 2990₽; 15 шт — 4490₽")
			return
		}
		if err := b.repo.ReplaceVariants(w.productID, variants); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		delete(b.wizards, chatID)
		b.send(chatID, "✅ Изменения сохранены.")
	case "edit_photos":
		b.send(chatID, "Отправьте фото (до 5 шт) или /done для завершения.")
	}
}

func (b *Bot) wizardAddText(chatID int64, w *wizard, text string) {
	switch w.step {
	case "name":
		w.draft.name = text
		w.step = "photos"
		b.send(chatID, "Шаг 2/5 — отправьте 4–5 фото букета (по одному). Когда закончите — /done.")
	case "photos":
		b.send(chatID, "Жду фото. Когда закончите — /done.")
	case "desc":
		if text != "-" {
			w.draft.description = text
		}
		w.step = "variants"
		b.send(chatID, "Шаг 4/5 — варианты и цены одной строкой.\nПример: 9 шт — 2990₽; 15 шт — 4490₽")
	case "variants":
		variants := parseVariants(text)
		if len(variants) == 0 {
			b.send(chatID, "Не понял формат. Пример: 9 шт — 2990₽; 15 шт — 4490₽")
			return
		}
		w.draft.variants = variants
		w.step = "category"
		b.sendKb(chatID, "Шаг 5/5 — выберите категорию:", categoryKeyboard("cat"))
	}
}

// wizardDone — /done: завершение приёма фото.
func (b *Bot) wizardDone(chatID int64) {
	w, ok := b.wizards[chatID]
	if !ok {
		b.send(chatID, "Сейчас нечего завершать.")
		return
	}
	switch {
	case w.mode == "add" && w.step == "photos":
		if len(w.draft.imageURLs) == 0 {
			b.send(chatID, "Нужно хотя бы одно фото.")
			return
		}
		w.step = "desc"
		b.send(chatID, "Шаг 3/5 — введите описание (или «-», чтобы пропустить):")
	case w.mode == "edit_photos":
		if len(w.draft.imageURLs) == 0 {
			delete(b.wizards, chatID)
			b.send(chatID, "Фото не получены, оставляю как было.")
			return
		}
		if err := b.repo.ReplaceImages(w.productID, w.draft.imageURLs); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		delete(b.wizards, chatID)
		b.send(chatID, "✅ Изменения сохранены.")
	default:
		b.send(chatID, "Сейчас нечего завершать.")
	}
}

func (b *Bot) savePhoto(photos []tgbotapi.PhotoSize) (string, error) {
	best := photos[len(photos)-1] // последний размер — самый большой
	fileURL, err := b.api.GetFileDirectURL(best.FileID)
	if err != nil {
		return "", err
	}
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	return b.store.Save("photo.jpg", resp.Body)
}

func categoryKeyboard(prefix string) tgbotapi.InlineKeyboardMarkup {
	emoji := []string{"💚", "💎", "✨", "⚡"}
	var rows [][]tgbotapi.InlineKeyboardButton
	for i, c := range model.Categories {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(emoji[i]+" "+c, fmt.Sprintf("%s:%d", prefix, i))))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// --- Callback-кнопки ---

func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	defer func() {
		if _, err := b.api.Request(tgbotapi.NewCallback(cb.ID, "")); err != nil {
			log.Printf("callback ack: %v", err)
		}
	}()

	chatID := cb.Message.Chat.ID
	if chatID != b.adminChatID {
		return
	}

	parts := strings.Split(cb.Data, ":")
	action := parts[0]
	argAt := func(i int) uint {
		if i >= len(parts) {
			return 0
		}
		v, _ := strconv.ParseUint(parts[i], 10, 32)
		return uint(v)
	}

	switch action {
	case "cat": // категория в визарде /add
		w, ok := b.wizards[chatID]
		if !ok || w.mode != "add" || w.step != "category" {
			return
		}
		idx := int(argAt(1))
		if idx >= len(model.Categories) {
			return
		}
		w.draft.category = model.Categories[idx]
		w.step = "confirm"
		d := w.draft
		summary := fmt.Sprintf("Проверьте товар:\n\n🌸 %s\nКатегория: %s\nОписание: %s\nФото: %d шт\nВарианты:\n%s",
			d.name, d.category, orDash(d.description), len(d.imageURLs), formatVariants(d.variants))
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Сохранить", "addok"),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "addcancel"),
		))
		b.sendKb(chatID, summary, kb)

	case "addok":
		w, ok := b.wizards[chatID]
		if !ok || w.mode != "add" || w.step != "confirm" {
			return
		}
		p := &model.Product{
			Name:        w.draft.name,
			Description: w.draft.description,
			Category:    w.draft.category,
			Variants:    w.draft.variants,
		}
		for _, u := range w.draft.imageURLs {
			p.Images = append(p.Images, model.ProductImage{URL: u})
		}
		if err := b.repo.CreateProduct(p); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		delete(b.wizards, chatID)
		b.send(chatID, fmt.Sprintf("✅ Товар «%s» добавлен (#%d) и уже виден в Mini App.", p.Name, p.ID))

	case "addcancel":
		delete(b.wizards, chatID)
		b.send(chatID, "Добавление отменено.")

	case "edit": // выбор товара для редактирования
		id := argAt(1)
		kb := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Название", fmt.Sprintf("editf:%d:name", id)),
				tgbotapi.NewInlineKeyboardButtonData("Описание", fmt.Sprintf("editf:%d:desc", id)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Цены", fmt.Sprintf("editf:%d:price", id)),
				tgbotapi.NewInlineKeyboardButtonData("Категория", fmt.Sprintf("editf:%d:cat", id)),
				tgbotapi.NewInlineKeyboardButtonData("Фото", fmt.Sprintf("editf:%d:photo", id)),
			),
		)
		b.sendKb(chatID, "Что меняем?", kb)

	case "editf":
		id := argAt(1)
		if len(parts) < 3 {
			return
		}
		switch parts[2] {
		case "name":
			b.wizards[chatID] = &wizard{mode: "edit_text", productID: id, field: "name"}
			b.send(chatID, "Введите новое название:")
		case "desc":
			b.wizards[chatID] = &wizard{mode: "edit_text", productID: id, field: "description"}
			b.send(chatID, "Введите новое описание (или «-», чтобы очистить):")
		case "price":
			b.wizards[chatID] = &wizard{mode: "edit_variants", productID: id}
			b.send(chatID, "Введите новые варианты одной строкой.\nПример: 9 шт — 2990₽; 15 шт — 4490₽")
		case "photo":
			b.wizards[chatID] = &wizard{mode: "edit_photos", productID: id}
			b.send(chatID, "Отправьте новые фото (до 5 шт) — они заменят старые. Когда закончите — /done.")
		case "cat":
			b.sendKb(chatID, "Выберите новую категорию:", categoryKeyboard(fmt.Sprintf("editcat_%d", id)))
		}

	case "hide":
		id := argAt(1)
		p, err := b.repo.GetProduct(id)
		if err != nil {
			b.send(chatID, "Товар не найден.")
			return
		}
		verb := "Скрыть"
		if p.IsHidden {
			verb = "Показать"
		}
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Да, "+strings.ToLower(verb), fmt.Sprintf("hideok:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "noop"),
		))
		b.sendKb(chatID, fmt.Sprintf("%s товар «%s»?", verb, p.Name), kb)

	case "hideok":
		id := argAt(1)
		p, err := b.repo.GetProduct(id)
		if err != nil {
			b.send(chatID, "Товар не найден.")
			return
		}
		if err := b.repo.SetProductHidden(id, !p.IsHidden); err != nil {
			b.send(chatID, "Ошибка: "+err.Error())
			return
		}
		if p.IsHidden {
			b.send(chatID, fmt.Sprintf("👁 Товар «%s» снова виден в каталоге.", p.Name))
		} else {
			b.send(chatID, fmt.Sprintf("🙈 Товар «%s» скрыт из каталога.", p.Name))
		}

	case "del":
		id := argAt(1)
		p, err := b.repo.GetProduct(id)
		if err != nil {
			b.send(chatID, "Товар не найден.")
			return
		}
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Да, удалить", fmt.Sprintf("delok:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "noop"),
		))
		b.sendKb(chatID, fmt.Sprintf("Удалить товар «%s» безвозвратно?", p.Name), kb)

	case "delok":
		id := argAt(1)
		if err := b.repo.DeleteProduct(id); err != nil {
			b.send(chatID, "Ошибка: "+err.Error())
			return
		}
		b.send(chatID, "🗑 Товар удалён.")

	case "order":
		b.sendOrderDetails(chatID, argAt(1))

	case "ost": // order set status: ost:<id>:<status>
		id := argAt(1)
		if len(parts) < 3 {
			return
		}
		status := parts[2]
		if _, ok := model.StatusLabels[status]; !ok {
			return
		}
		if err := b.repo.UpdateOrderStatus(id, status); err != nil {
			b.send(chatID, "Ошибка: "+err.Error())
			return
		}
		b.send(chatID, fmt.Sprintf("Заказ #%d → %s", id, model.StatusLabels[status]))

	case "ostmenu": // выбор статуса
		id := argAt(1)
		var rows [][]tgbotapi.InlineKeyboardButton
		for _, st := range model.StatusOrder {
			rows = append(rows, tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(model.StatusLabels[st], fmt.Sprintf("ost:%d:%s", id, st))))
		}
		b.sendKb(chatID, fmt.Sprintf("Новый статус заказа #%d:", id), tgbotapi.NewInlineKeyboardMarkup(rows...))

	case "noop":
		// отмена подтверждения — ничего не делаем

	default:
		// editcat_<id>:<catIdx>
		if strings.HasPrefix(action, "editcat_") {
			id64, _ := strconv.ParseUint(strings.TrimPrefix(action, "editcat_"), 10, 32)
			idx := int(argAt(1))
			if idx >= len(model.Categories) {
				return
			}
			p, err := b.repo.GetProduct(uint(id64))
			if err != nil {
				b.send(chatID, "Товар не найден.")
				return
			}
			p.Category = model.Categories[idx]
			if err := b.repo.SaveProduct(p); err != nil {
				b.send(chatID, "Ошибка: "+err.Error())
				return
			}
			b.send(chatID, "✅ Изменения сохранены.")
		}
	}
}

// --- Списки и карточки ---

func (b *Bot) sendProductList(chatID int64, title, action string) {
	products, err := b.repo.ListAllProducts()
	if err != nil || len(products) == 0 {
		b.send(chatID, "Товаров пока нет. Добавьте первый: /add")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, p := range products {
		label := fmt.Sprintf("#%d %s", p.ID, p.Name)
		if p.IsHidden {
			label += " 🙈"
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("%s:%d", action, p.ID))))
		if len(rows) >= 20 {
			break
		}
	}
	b.sendKb(chatID, title, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func (b *Bot) sendOrderList(chatID int64) {
	orders, err := b.repo.ListRecentOrders(10)
	if err != nil || len(orders) == 0 {
		b.send(chatID, "Заказов пока нет.")
		return
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, o := range orders {
		label := fmt.Sprintf("#%d · %d₽ · %s", o.ID, o.TotalPrice, model.StatusLabels[o.Status])
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("order:%d", o.ID))))
	}
	b.sendKb(chatID, "Последние заказы:", tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func (b *Bot) sendOrderDetails(chatID int64, id uint) {
	o, err := b.repo.GetOrder(id)
	if err != nil {
		b.send(chatID, "Заказ не найден.")
		return
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Подтвердить", fmt.Sprintf("ost:%d:%s", o.ID, model.StatusConfirmed)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", fmt.Sprintf("ost:%d:%s", o.ID, model.StatusCancelled)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Изменить статус", fmt.Sprintf("ostmenu:%d", o.ID)),
		),
	)
	b.sendKb(chatID, formatOrder(o, false), kb)
}

// NotifyNewOrder шлёт админу уведомление о новом заказе.
func (b *Bot) NotifyNewOrder(o *model.Order) {
	if b.adminChatID == 0 {
		return
	}
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Подтвердить", fmt.Sprintf("ost:%d:%s", o.ID, model.StatusConfirmed)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", fmt.Sprintf("ost:%d:%s", o.ID, model.StatusCancelled)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Изменить статус", fmt.Sprintf("ostmenu:%d", o.ID)),
		),
	)
	b.sendKb(b.adminChatID, formatOrder(o, true), kb)
}

func formatOrder(o *model.Order, isNew bool) string {
	var sb strings.Builder
	if isNew {
		fmt.Fprintf(&sb, "🌸 Новый заказ #%d\n", o.ID)
	} else {
		fmt.Fprintf(&sb, "🌸 Заказ #%d · %s\n", o.ID, model.StatusLabels[o.Status])
	}
	fmt.Fprintf(&sb, "Клиент: %s\n", o.User.Name)
	fmt.Fprintf(&sb, "Телефон: %s\n", o.User.Phone)
	sb.WriteString("Букеты: ")
	for i, it := range o.Items {
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
	if o.PromoCode != nil {
		fmt.Fprintf(&sb, "Промокод: %s (−%d%%)\n", o.PromoCode.Code, o.PromoCode.DiscountPercent)
	}
	fmt.Fprintf(&sb, "Итого: %d₽\n", o.TotalPrice)
	if o.Comment != "" {
		fmt.Fprintf(&sb, "Комментарий: %s", o.Comment)
	}
	return strings.TrimSpace(sb.String())
}

func formatVariants(vs []model.ProductVariant) string {
	var lines []string
	for _, v := range vs {
		lines = append(lines, fmt.Sprintf("  • %d шт — %d₽", v.Quantity, v.Price))
	}
	return strings.Join(lines, "\n")
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
