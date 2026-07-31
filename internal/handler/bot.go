package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/observability"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

type Bot struct {
	api      *tgbotapi.BotAPI
	repo     *repository.Repository
	svc      *service.Service
	store    storage.Storage
	adminIDs []int64 // whitelist: команды и callback-кнопки проверяются по нему
	appURL   string

	// OnBackup — ручной запуск бэкапа по /backup (ставится в main).
	OnBackup func(chatID int64)

	// fallback — диалоги заказа в чате, когда Mini App недоступен.
	fallback *fallbackState

	// Канал магазина (env CHANNEL_ID): /post и промокод за подписку.
	channelChatID   int64
	channelUsername string

	// limiter выстраивает все обращения к Bot API в очередь под лимиты Telegram.
	// Он один на бота: и оперативные уведомления, и рассылки, и посты в канал
	// делят общую квоту Telegram, поэтому и считать её нужно в одном месте.
	limiter tgLimiter

	// Состояние визардов /add и /edit — по chat_id. В режиме webhook апдейты
	// приходят параллельно, поэтому доступ под мьютексом.
	wizards map[int64]*wizard
	wizMu   sync.Mutex

	// Журнал сообщений в админ-чатах для /clean. Мьютекс нужен:
	// NotifyNewOrder пишет из горутины сервиса.
	msgLog map[int64][]int
	logMu  sync.Mutex

	// Рассылки идут строго по одной: лимит Telegram общий с уведомлениями
	// о заказах, и две параллельные упёрлись бы в 429.
	bcastMu sync.Mutex

	// Учёт незавершённых отправок для graceful shutdown: по SIGTERM сервис
	// перестаёт принимать апдейты и ждёт, пока начатые ответы уйдут клиенту.
	inflight sync.WaitGroup
	stopping atomic.Bool
}

// trackStart регистрирует начатую фоновую задачу бота.
// false — сервис останавливается, новую работу начинать нельзя.
func (b *Bot) trackStart() bool {
	if b.stopping.Load() {
		return false
	}
	b.inflight.Add(1)
	return true
}

func (b *Bot) trackDone() { b.inflight.Done() }

func (b *Bot) capturePanic(rec any, component string) {
	observability.CapturePanic(rec, map[string]string{"component": "bot:" + component})
}

// dispatch направляет апдейт в нужный обработчик (общий для polling и webhook).
func (b *Bot) dispatch(update tgbotapi.Update) {
	switch {
	case update.CallbackQuery != nil:
		b.handleCallback(update.CallbackQuery)
	case update.Message != nil:
		b.handleMessage(update.Message)
	}
}

// Stop останавливает приём апдейтов и ждёт, пока завершатся начатые отправки
// (или пока не истечёт ctx). Вызывается из graceful shutdown в main.
func (b *Bot) Stop(ctx context.Context) {
	if b.stopping.Swap(true) {
		return
	}
	b.api.StopReceivingUpdates()

	done := make(chan struct{})
	go func() {
		b.inflight.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Println("бот: незавершённых отправок нет")
	case <-ctx.Done():
		log.Println("бот: не дождались отправок, выходим по таймауту")
	}
}

// API даёт доступ к Bot API для проб живости и отправки бэкапов.
func (b *Bot) API() *tgbotapi.BotAPI { return b.api }

// Username — @имя бота (для кнопки «открыть чат с ботом» в Mini App).
func (b *Bot) Username() string { return b.api.Self.UserName }

// --- Состояние визардов ---

func (b *Bot) getWizard(chatID int64) (*wizard, bool) {
	b.wizMu.Lock()
	defer b.wizMu.Unlock()
	w, ok := b.wizards[chatID]
	return w, ok
}

func (b *Bot) setWizard(chatID int64, w *wizard) {
	b.wizMu.Lock()
	b.wizards[chatID] = w
	b.wizMu.Unlock()
}

func (b *Bot) clearWizard(chatID int64) {
	b.wizMu.Lock()
	delete(b.wizards, chatID)
	b.wizMu.Unlock()
}

const msgLogCap = 400 // сколько последних сообщений помним на чат

// remember сохраняет id сообщения в админ-чате, чтобы /clean мог его удалить.
func (b *Bot) remember(chatID int64, msgID int) {
	if msgID == 0 || !b.isAdmin(chatID) {
		return
	}
	b.logMu.Lock()
	defer b.logMu.Unlock()
	ids := append(b.msgLog[chatID], msgID)
	if len(ids) > msgLogCap {
		ids = ids[len(ids)-msgLogCap:]
	}
	b.msgLog[chatID] = ids
}

func (b *Bot) isAdmin(id int64) bool {
	for _, a := range b.adminIDs {
		if a == id {
			return true
		}
	}
	return false
}

type wizard struct {
	mode      string // add | edit_text | edit_variants | edit_photos | edit_discount | edit_stock | fresh | order_photo | cancel_reason | broadcast_content
	step      string // для add: name → photos → desc → variants → category → confirm
	productID uint   // для edit
	orderID   uint   // для order_photo / cancel_reason
	msgID     int    // сообщение-карточка, которое редактируем после действия
	field     string // name | description
	segment   string // для broadcast_content: выбранный сегмент аудитории
	draft     draft
}

type draft struct {
	name        string
	description string
	category    string
	variants    []model.ProductVariant
	imageURLs   []string
}

func NewBot(token string, adminIDs []int64, appURL, channelID string, repo *repository.Repository, svc *service.Service, store storage.Storage) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, err
	}
	// Хостинг может отдавать адрес без схемы (Render: flowix.onrender.com),
	// а web_app-кнопке нужен полный https-URL.
	appURL = strings.TrimSuffix(strings.TrimSpace(appURL), "/")
	if appURL != "" && !strings.Contains(appURL, "://") {
		appURL = "https://" + appURL
	}
	chatID, username := parseChannelID(channelID)
	b := &Bot{
		api:             api,
		repo:            repo,
		svc:             svc,
		store:           store,
		adminIDs:        adminIDs,
		appURL:          appURL,
		channelChatID:   chatID,
		channelUsername: username,
		wizards:         map[int64]*wizard{},
		msgLog:          map[int64][]int{},
		fallback:        newFallbackState(),
	}
	svc.NotifyNewOrder = b.NotifyNewOrder
	b.syncMenuButton()
	return b, nil
}

// syncMenuButton делает кнопку меню бота постоянной ссылкой на Mini App.
// Без неё магазин открывается только из inline-кнопки конкретного сообщения,
// а в старых сообщениях адрес вшит навсегда — после переезда они ведут в никуда.
// Метод появился в Bot API 6.0, в tgbotapi v5.5.1 его нет — зовём напрямую.
func (b *Bot) syncMenuButton() {
	if b.appURL == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"menu_button": map[string]any{
			"type":    "web_app",
			"text":    "🌸 Магазин",
			"web_app": map[string]string{"url": b.appURL},
		},
	})
	if err != nil {
		return
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/setChatMenuButton", b.api.Token)
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("menu button: %v", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("menu button: HTTP %d", resp.StatusCode)
		return
	}
	log.Printf("кнопка меню бота ведёт на %s", b.appURL)
}

func (b *Bot) Run() {
	log.Printf("бот запущен: @%s", b.api.Self.UserName)
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	for update := range b.api.GetUpdatesChan(u) {
		if !b.trackStart() {
			return // получен SIGTERM: новые апдейты не разбираем
		}
		func() {
			defer b.trackDone()
			defer func() {
				if r := recover(); r != nil {
					b.capturePanic(r, "polling")
				}
			}()
			b.dispatch(update)
		}()
	}
}

func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	m, err := b.tgSend(msg)
	if err != nil {
		log.Printf("bot send: %v", err)
		return
	}
	b.remember(chatID, m.MessageID)
}

func (b *Bot) sendKb(chatID int64, text string, kb tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = kb
	m, err := b.tgSend(msg)
	if err != nil {
		log.Printf("bot send: %v", err)
		return
	}
	b.remember(chatID, m.MessageID)
}

// sendTemp — служебное сообщение, которое самоуничтожается через ttl.
func (b *Bot) sendTemp(chatID int64, text string, ttl time.Duration) {
	m, err := b.tgSend(tgbotapi.NewMessage(chatID, text))
	if err != nil {
		log.Printf("bot send: %v", err)
		return
	}
	time.AfterFunc(ttl, func() {
		if _, err := b.tgRequest(tgbotapi.NewDeleteMessage(chatID, m.MessageID)); err != nil {
			log.Printf("bot temp delete: %v", err)
		}
	})
}

// cleanChat — /clean: удаляет все запомненные сообщения диалога
// (Telegram позволяет удалять сообщения младше 48 часов; старые пропускаем).
func (b *Bot) cleanChat(chatID int64) {
	b.logMu.Lock()
	ids := b.msgLog[chatID]
	delete(b.msgLog, chatID)
	b.logMu.Unlock()

	deleted := 0
	for _, id := range ids {
		if _, err := b.tgRequest(tgbotapi.NewDeleteMessage(chatID, id)); err == nil {
			deleted++
		}
	}
	b.sendTemp(chatID, fmt.Sprintf("🧹 Убрано %d сообщений.", deleted), 4*time.Second)
}

// editKb — правит текст и клавиатуру существующего сообщения (чистый чат вместо спама).
func (b *Bot) editKb(chatID int64, msgID int, text string, kb tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, kb)
	if _, err := b.tgSend(edit); err != nil {
		log.Printf("bot edit: %v", err)
	}
}

// --- Входящие сообщения ---

func (b *Bot) handleMessage(msg *tgbotapi.Message) {
	// Не-админы видят обычный магазин: никаких намёков на админ-команды.
	if !b.isAdmin(msg.Chat.ID) {
		b.handleCustomer(msg)
		return
	}

	// Помним и входящие сообщения админа — /clean уберёт и их.
	b.remember(msg.Chat.ID, msg.MessageID)

	if msg.IsCommand() {
		switch msg.Command() {
		case "start", "help":
			b.send(msg.Chat.ID, "Команды администратора:\n"+
				"/orders — заказы по статусам\n"+
				"/preorders — 📅 предзаказы (доставка позже сегодня)\n"+
				"/clients <имя или телефон> — база клиентов\n"+
				"/stats — 📊 сводка за период\n"+
				"/add — добавить товар\n"+
				"/edit — изменить товар\n"+
				"/stock — 📦 наличие товаров\n"+
				"/hide — скрыть/показать товар\n"+
				"/delete — удалить товар\n"+
				"/broadcast — 📣 рассылка по клиентам\n"+
				"/export — 📄 выгрузка заказов в CSV\n"+
				"/fresh — что сегодня свежее на базе\n"+
				"/post <id товара> — 📣 пост о товаре в канал\n"+
				"/fallback on|off — заказ через диалог бота (если Mini App лежит)\n"+
				"/backup — 💾 выгрузить дамп БД прямо сейчас\n"+
				"/clean — 🧹 очистить историю чата\n"+
				"/cancel — прервать текущее действие")
		case "fallback":
			b.setFallback(msg.Chat.ID, msg.CommandArguments())
		case "backup":
			if b.OnBackup == nil {
				b.send(msg.Chat.ID, "Бэкапы не настроены: задайте BACKUP_CHANNEL_ID (см. README).")
				return
			}
			b.send(msg.Chat.ID, "💾 Делаю дамп базы — пришлю файл, как будет готов.")
			chatID := msg.Chat.ID
			if b.trackStart() {
				observability.GoSafe("backup:manual", func() {
					defer b.trackDone()
					b.OnBackup(chatID)
				})
			}
		case "fresh":
			if items := strings.TrimSpace(msg.CommandArguments()); items != "" {
				b.saveFresh(msg.Chat.ID, items)
				return
			}
			b.setWizard(msg.Chat.ID, &wizard{mode: "fresh"})
			b.send(msg.Chat.ID, "🌷 Что сегодня свежее? Пришлите одной строкой, например:\nпионы, ранункулюсы, эустома")
		case "add":
			b.setWizard(msg.Chat.ID, &wizard{mode: "add", step: "name"})
			b.send(msg.Chat.ID, "🌸 Новый товар.\n\nШаг 1/5 — введите название:")
		case "edit":
			b.sendProductList(msg.Chat.ID, "Что редактируем?", "edit")
		case "hide":
			b.sendProductList(msg.Chat.ID, "Какой товар скрыть/показать?", "hide")
		case "delete":
			b.sendProductList(msg.Chat.ID, "Какой товар удалить?", "del")
		case "stock":
			b.sendStock(msg.Chat.ID)
		case "stats":
			b.sendStats(msg.Chat.ID)
		case "broadcast":
			b.sendBroadcastSegments(msg.Chat.ID)
		case "export":
			b.sendExportMenu(msg.Chat.ID)
		case "post":
			b.handlePostCommand(msg.Chat.ID, msg.CommandArguments())
		case "orders":
			b.sendStatusFilter(msg.Chat.ID)
		case "preorders":
			b.sendPreorders(msg.Chat.ID)
		case "clients":
			b.sendClientSearch(msg.Chat.ID, msg.CommandArguments())
		case "done":
			b.wizardDone(msg.Chat.ID)
		case "clean":
			b.cleanChat(msg.Chat.ID)
		case "cancel":
			b.clearWizard(msg.Chat.ID)
			b.sendTemp(msg.Chat.ID, "Действие отменено.", 4*time.Second)
		default:
			b.send(msg.Chat.ID, "Неизвестная команда. /help — список команд.")
		}
		return
	}

	if w, ok := b.getWizard(msg.Chat.ID); ok {
		b.wizardInput(msg, w)
		return
	}
	b.send(msg.Chat.ID, "Используйте команды: /add /edit /hide /delete /orders")
}

// handleCustomer — не-админ: deep-link промокоды, ответы-отзывы, кнопка Mini App
// и заказ диалогом, если Mini App недоступен.
func (b *Bot) handleCustomer(msg *tgbotapi.Message) {
	// Регистрируем всех, кто написал боту: сегмент «Все» в /broadcast обещает
	// именно это. Имя и телефон не заполняем — они точнее приходят из checkout.
	if msg.From != nil {
		if _, err := b.repo.UpsertUser(msg.From.ID, "", ""); err != nil {
			log.Printf("upsert user: %v", err)
		}
	}

	if msg.IsCommand() {
		switch msg.Command() {
		case "order":
			b.handleFallbackCommand(msg.Chat.ID)
			return
		case "cancel":
			b.fallback.clear(msg.Chat.ID)
			b.send(msg.Chat.ID, "Хорошо, отменил. Начать заказ заново — /order")
			return
		case "start":
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
	}

	// Идёт диалог заказа — ответ клиента относится к нему. Проверяем раньше
	// отзыва: пока клиент диктует адрес, его сообщения — это заказ, а не
	// ответ на висящий с прошлой доставки вопрос «Как вам букет?».
	if b.handleFallbackInput(msg) {
		return
	}
	// Любой текст в течение 48 часов после вопроса «Как вам букет?» — отзыв:
	// пересылаем админам и дарим одноразовый промокод.
	if !msg.IsCommand() && strings.TrimSpace(msg.Text) != "" && b.handleFeedbackReply(msg) {
		return
	}
	b.sendShopButton(msg.Chat.ID, "🌸 Добро пожаловать в Flowix!\nВыбирайте букеты в нашем магазине:")
}

// handleFeedbackReply — текст клиента в окне отзыва (48ч после вопроса):
// true, если сообщение обработано как отзыв. Каждое сообщение пересылается
// админам, промокод выдаётся только за первое (guard — responded_at).
func (b *Bot) handleFeedbackReply(msg *tgbotapi.Message) bool {
	n, err := b.repo.OpenFeedback(msg.From.ID, model.FeedbackWindow)
	if err != nil {
		return false // открытого окна отзыва нет
	}
	o, err := b.repo.GetOrder(n.OrderID)
	if err != nil {
		log.Printf("отзыв: заказ #%d не найден: %v", n.OrderID, err)
		return false
	}

	// Пересылаем отзыв всем админам.
	report := fmt.Sprintf("%s\nОт: %s · %s\n\n%s",
		fmt.Sprintf(model.AdminFeedbackHeader, o.ID), orDash(o.User.Name), orDash(o.User.Phone), msg.Text)
	for _, adminID := range b.adminIDs {
		b.send(adminID, report)
	}

	// Промокод — только за первый ответ: responded_at ставится атомарно один раз.
	first, err := b.repo.MarkFeedbackResponded(n.ID, time.Now())
	if err != nil {
		log.Printf("отметка отзыва по заказу #%d: %v", o.ID, err)
	}
	if !first {
		b.send(msg.Chat.ID, model.MsgFeedbackMore)
		return true
	}
	promo, err := b.repo.CreateSingleUsePromo("OTZYV", model.FeedbackPromoPercent)
	if err != nil {
		log.Printf("промокод за отзыв (заказ #%d): %v", o.ID, err)
		b.send(msg.Chat.ID, model.MsgFeedbackMore)
		return true
	}
	b.send(msg.Chat.ID, fmt.Sprintf(model.MsgFeedbackThanks, promo.DiscountPercent, promo.Code))
	return true
}

// webAppKeyboard — inline-клавиатура с кнопкой web_app (запуск Mini App).
// В tgbotapi v5.5.1 поля web_app нет, поэтому собираем JSON-совместимую
// структуру сами: библиотека сериализует ReplyMarkup через json.Marshal как есть.
type webAppKeyboard struct {
	InlineKeyboard [][]webAppButton `json:"inline_keyboard"`
}

type webAppButton struct {
	Text         string     `json:"text"`
	WebApp       *webAppURL `json:"web_app,omitempty"`
	CallbackData string     `json:"callback_data,omitempty"`
}

type webAppURL struct {
	URL string `json:"url"`
}

// sendShopButton — приглашение в магазин. Если Mini App не настроен или
// включён fallback-режим, рядом (или вместо) появляется кнопка заказа в чате:
// клиент не должен упереться в тупик, когда приложение недоступно.
func (b *Bot) sendShopButton(chatID int64, text string) {
	var row []webAppButton
	if b.appURL != "" {
		row = append(row, webAppButton{Text: "🌸 Открыть магазин", WebApp: &webAppURL{URL: b.appURL}})
	}
	if b.fallbackEnabled() {
		row = append(row, webAppButton{Text: "💬 Заказать в чате", CallbackData: "forder"})
	}
	if len(row) == 0 {
		b.send(chatID, text)
		return
	}
	if b.appURL == "" {
		text += "\n\nПриложение магазина сейчас недоступно — оформим заказ прямо здесь."
	}

	msg := tgbotapi.NewMessage(chatID, text)
	// Кнопки в столбик: подписи длинные, в одну строку не помещаются.
	rows := make([][]webAppButton, 0, len(row))
	for _, btn := range row {
		rows = append(rows, []webAppButton{btn})
	}
	msg.ReplyMarkup = webAppKeyboard{InlineKeyboard: rows}
	if _, err := b.tgSend(msg); err != nil {
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

	// Рассылка принимает и сжатое фото, и текст — разбираем её раньше общих
	// правил приёма фото, которые защищают качество витрины.
	if w.mode == "broadcast_content" {
		b.broadcastContent(msg, w)
		return
	}

	// Фото букета по заказу: принимаем и обычное фото, и файл — храним только
	// telegram file_id, пересылаем клиенту без скачивания.
	if w.mode == "order_photo" && (len(msg.Photo) > 0 || msg.Document != nil) {
		var fileID string
		var isDoc bool
		switch {
		case len(msg.Photo) > 0:
			fileID = msg.Photo[len(msg.Photo)-1].FileID // последний размер — самый крупный
		case isImageDocument(msg.Document):
			fileID, isDoc = msg.Document.FileID, true
		default:
			b.send(chatID, "Это не изображение. Пришлите фото букета или /cancel.")
			return
		}
		b.clearWizard(chatID)
		b.handleOrderPhoto(chatID, w, fileID, isDoc)
		return
	}

	// Сжатое фото товара не принимаем: Telegram ужимает его до ~1280px, на витрине это мыло.
	if len(msg.Photo) > 0 {
		b.send(chatID, sendAsFileHint)
		return
	}

	// Приём фото товара файлом (шаг photos в /add или замена фото в /edit).
	if msg.Document != nil {
		if (w.mode == "add" && w.step == "photos") || w.mode == "edit_photos" {
			if !isImageDocument(msg.Document) {
				b.send(chatID, "Это не изображение. "+sendAsFileHint)
				return
			}
			url, err := b.saveDocument(msg.Document)
			if err != nil {
				log.Printf("save photo: %v", err)
				b.send(chatID, "Не удалось сохранить фото: "+err.Error())
				return
			}
			w.draft.imageURLs = append(w.draft.imageURLs, url)
			n := len(w.draft.imageURLs)
			if n >= 5 {
				b.wizardDone(chatID)
				return
			}
			b.send(chatID, fmt.Sprintf("🖼 Фото %d/5 сохранено в оригинале. Отправьте ещё или /done.", n))
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
	case "fresh":
		b.clearWizard(chatID)
		b.saveFresh(chatID, text)
	case "cancel_reason":
		b.clearWizard(chatID)
		updated, err := b.svc.TransitionOrder(w.orderID, model.StatusCancelled, msg.From.ID, text)
		if err != nil {
			b.send(chatID, "Не получилось отменить: "+err.Error())
			return
		}
		// Обновляем исходную карточку на месте и подтверждаем.
		if w.msgID != 0 {
			b.editOrderCard(chatID, w.msgID, updated)
		}
		b.send(chatID, fmt.Sprintf("❌ Заказ #%d отменён: %s", updated.ID, text))
		b.notifyCustomerStatus(updated.ID)
	case "order_photo":
		b.send(chatID, "Жду фото букета. Или /cancel для отмены.")
	case "edit_text":
		p, err := b.repo.GetProduct(w.productID)
		if err != nil {
			b.clearWizard(chatID)
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
		b.clearWizard(chatID)
		b.sendTemp(chatID, "✅ Изменения сохранены.", 5*time.Second)
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
		b.clearWizard(chatID)
		b.sendTemp(chatID, "✅ Изменения сохранены.", 5*time.Second)
	case "edit_discount":
		pct, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || pct < 0 || pct > 99 {
			b.send(chatID, "Нужно число от 0 до 99. Попробуйте ещё раз или /cancel.")
			return
		}
		if err := b.repo.SetProductDiscount(w.productID, pct); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		b.clearWizard(chatID)
		if pct == 0 {
			b.sendTemp(chatID, "✅ Скидка убрана.", 5*time.Second)
		} else {
			b.send(chatID, fmt.Sprintf("✅ Скидка −%d%% включена, на витрине появится бейдж.", pct))
		}
	case "edit_stock":
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 0 || n > 9999 {
			b.send(chatID, "Нужно число от 0 до 9999. Попробуйте ещё раз или /cancel.")
			return
		}
		if err := b.repo.SetProductStock(w.productID, n); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		b.clearWizard(chatID)
		if n == 0 {
			b.sendTemp(chatID, "✅ Бейдж «осталось N» скрыт.", 5*time.Second)
		} else {
			b.send(chatID, fmt.Sprintf("✅ Остаток %d — бейдж появится, когда ≤ 5.", n))
		}
	case "edit_sort":
		n, err := strconv.Atoi(strings.TrimSpace(text))
		if err != nil || n < 0 || n > 9999 {
			b.send(chatID, "Нужно число от 0 до 9999. Попробуйте ещё раз или /cancel.")
			return
		}
		if err := b.repo.SetProductSortOrder(w.productID, n); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		b.clearWizard(chatID)
		b.sendTemp(chatID, fmt.Sprintf("✅ Порядок в хитах: %d.", n), 5*time.Second)
	case "edit_photos":
		b.send(chatID, "Отправьте фото файлом (до 5 шт) или /done для завершения.")
	}
}

func (b *Bot) wizardAddText(chatID int64, w *wizard, text string) {
	switch w.step {
	case "name":
		w.draft.name = text
		w.step = "photos"
		b.send(chatID, "Шаг 2/5 — отправьте 4–5 фото букета по одному.\n\n"+sendAsFileHint+"\n\nКогда закончите — /done.")
	case "photos":
		b.send(chatID, "Жду фото файлом. Когда закончите — /done.")
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
	w, ok := b.getWizard(chatID)
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
			b.clearWizard(chatID)
			b.send(chatID, "Фото не получены, оставляю как было.")
			return
		}
		if err := b.repo.ReplaceImages(w.productID, w.draft.imageURLs); err != nil {
			b.send(chatID, "Ошибка сохранения: "+err.Error())
			return
		}
		b.clearWizard(chatID)
		b.sendTemp(chatID, "✅ Изменения сохранены.", 5*time.Second)
	default:
		b.send(chatID, "Сейчас нечего завершать.")
	}
}

// sendAsFileHint — единая подсказка, как прислать фото без потери качества.
const sendAsFileHint = "Пришлите фото файлом, без сжатия:\n" +
	"📎 → Файл (не «Фото») → выберите снимок.\n" +
	"На iPhone: 📎 → Файл; если выбираете из галереи — сначала «Сохранить в Файлы».\n" +
	"Так на витрину попадёт оригинал, а не сжатая Telegram копия."

// maxDocumentBytes — предел скачивания файла ботом в Telegram Bot API.
const maxDocumentBytes = 20 << 20

func isImageDocument(doc *tgbotapi.Document) bool {
	if strings.HasPrefix(doc.MimeType, "image/") {
		return true
	}
	// Некоторые клиенты не проставляют MIME — смотрим на расширение.
	switch strings.ToLower(filepath.Ext(doc.FileName)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".heic", ".heif":
		return true
	}
	return false
}

// saveDocument скачивает присланный файлом снимок и кладёт в хранилище как есть —
// без сжатия, в оригинальном разрешении.
func (b *Bot) saveDocument(doc *tgbotapi.Document) (string, error) {
	if doc.FileSize > maxDocumentBytes {
		return "", fmt.Errorf("файл больше 20 МБ — Telegram не отдаёт такие ботам")
	}
	fileURL, err := b.api.GetFileDirectURL(doc.FileID)
	if err != nil {
		return "", err
	}
	resp, err := http.Get(fileURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Telegram вернул %d при скачивании", resp.StatusCode)
	}
	name := doc.FileName
	if name == "" {
		name = "photo.jpg"
	}
	return b.store.Save(name, resp.Body)
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
		if _, err := b.tgRequest(tgbotapi.NewCallback(cb.ID, "")); err != nil {
			log.Printf("callback ack: %v", err)
		}
	}()

	chatID := cb.Message.Chat.ID

	// Кнопки диалога заказа доступны всем — это клиентский сценарий.
	if isFallbackCallback(cb.Data) {
		if cb.Data == "fok" {
			var tgID int64
			if cb.From != nil {
				tgID = cb.From.ID
			}
			b.confirmFallbackOrder(chatID, tgID)
			return
		}
		b.handleFallbackCallback(cb)
		return
	}

	// Остальные callback-кнопки — админские, проверяем по whitelist:
	// payload можно подделать.
	if !b.isAdmin(chatID) || (cb.From != nil && !b.isAdmin(cb.From.ID)) {
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
		w, ok := b.getWizard(chatID)
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
		w, ok := b.getWizard(chatID)
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
		b.clearWizard(chatID)
		b.send(chatID, fmt.Sprintf("✅ Товар «%s» добавлен (#%d) и уже виден в Mini App.", p.Name, p.ID))

	case "addcancel":
		b.clearWizard(chatID)
		b.sendTemp(chatID, "Добавление отменено.", 5*time.Second)

	case "edit": // выбор товара для редактирования
		id := argAt(1)
		p, _ := b.repo.GetProduct(id)
		hitLabel := "⭐ Хит: выкл"
		if p != nil && p.IsHit {
			hitLabel = "⭐ Хит: вкл"
		}
		lowLabel := "🔻 Мало осталось: выкл"
		if p != nil && p.LowStock {
			lowLabel = "🔻 Мало осталось: вкл"
		}
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
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(hitLabel, fmt.Sprintf("editf:%d:hit", id)),
				tgbotapi.NewInlineKeyboardButtonData("🏷 Скидка", fmt.Sprintf("editf:%d:disc", id)),
				tgbotapi.NewInlineKeyboardButtonData("📦 Остаток", fmt.Sprintf("editf:%d:stock", id)),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(lowLabel, fmt.Sprintf("editf:%d:low", id)),
				tgbotapi.NewInlineKeyboardButtonData("🔢 Порядок в хитах", fmt.Sprintf("editf:%d:sort", id)),
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
			b.setWizard(chatID, &wizard{mode: "edit_text", productID: id, field: "name"})
			b.send(chatID, "Введите новое название:")
		case "desc":
			b.setWizard(chatID, &wizard{mode: "edit_text", productID: id, field: "description"})
			b.send(chatID, "Введите новое описание (или «-», чтобы очистить):")
		case "price":
			b.setWizard(chatID, &wizard{mode: "edit_variants", productID: id})
			b.send(chatID, "Введите новые варианты одной строкой.\nПример: 9 шт — 2990₽; 15 шт — 4490₽")
		case "photo":
			b.setWizard(chatID, &wizard{mode: "edit_photos", productID: id})
			b.send(chatID, "Новые фото заменят старые (до 5 шт).\n\n"+sendAsFileHint+"\n\nКогда закончите — /done.")
		case "cat":
			b.sendKb(chatID, "Выберите новую категорию:", categoryKeyboard(fmt.Sprintf("editcat_%d", id)))
		case "hit":
			p, err := b.repo.GetProduct(id)
			if err != nil {
				b.send(chatID, "Товар не найден.")
				return
			}
			if err := b.repo.SetProductHit(id, !p.IsHit); err != nil {
				b.send(chatID, "Ошибка: "+err.Error())
				return
			}
			if p.IsHit {
				b.sendTemp(chatID, "⭐ Бейдж «ХИТ» убран.", 5*time.Second)
			} else {
				b.sendTemp(chatID, "⭐ Бейдж «ХИТ» включён.", 5*time.Second)
			}
		case "low":
			p, err := b.repo.GetProduct(id)
			if err != nil {
				b.send(chatID, "Товар не найден.")
				return
			}
			if err := b.repo.SetProductLowStock(id, !p.LowStock); err != nil {
				b.send(chatID, "Ошибка: "+err.Error())
				return
			}
			if p.LowStock {
				b.sendTemp(chatID, "🔻 Бейдж «мало осталось» убран.", 5*time.Second)
			} else {
				b.sendTemp(chatID, "🔻 Бейдж «мало осталось» включён.", 5*time.Second)
			}
		case "sort":
			b.setWizard(chatID, &wizard{mode: "edit_sort", productID: id})
			b.send(chatID, "Введите порядок в подборке хитов: меньше — выше (например 10). 0 — по умолчанию.")
		case "disc":
			b.setWizard(chatID, &wizard{mode: "edit_discount", productID: id})
			b.send(chatID, "Введите процент скидки (например 10). 0 — убрать скидку.")
		case "stock":
			b.setWizard(chatID, &wizard{mode: "edit_stock", productID: id})
			b.send(chatID, "Введите остаток для бейджа «осталось N» (например 3). 0 — скрыть бейдж.")
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
		b.sendKb(chatID, fmt.Sprintf("Убрать товар «%s» из каталога?\n\nОн исчезнет с витрины, но останется в истории прошлых заказов.", p.Name), kb)

	case "delok":
		id := argAt(1)
		if err := b.repo.DeleteProduct(id); err != nil {
			b.send(chatID, "Ошибка: "+err.Error())
			return
		}
		b.sendTemp(chatID, "🗑 Товар убран из каталога.", 5*time.Second)

	case "st": // st:<period> — отчёт /stats за период, правим то же сообщение
		if len(parts) < 2 {
			return
		}
		text, kb := b.buildStats(parts[1])
		b.editKb(chatID, cb.Message.MessageID, text, kb)

	case "stk": // stk:<productID>:<page> — переключить наличие товара
		if len(parts) < 3 {
			return
		}
		b.toggleStock(chatID, cb.Message.MessageID, argAt(1), int(argAt(2)))

	case "stkpg": // stkpg:<page> — страница списка наличия
		b.renderStockPage(chatID, cb.Message.MessageID, int(argAt(1)))

	case "bcseg": // bcseg:<segment> — выбран сегмент рассылки
		if len(parts) < 2 {
			return
		}
		if _, ok := model.SegmentLabels[parts[1]]; !ok {
			return
		}
		b.startBroadcastContent(chatID, parts[1])

	case "bc": // bc:<id>:go|no — подтверждение или отмена рассылки
		if len(parts) < 3 {
			return
		}
		switch parts[2] {
		case "go":
			b.confirmBroadcast(chatID, cb.Message.MessageID, argAt(1))
		case "no":
			b.cancelBroadcast(chatID, cb.Message.MessageID, argAt(1))
		}

	case "exp": // exp:<period> — CSV-выгрузка заказов
		if len(parts) < 2 {
			return
		}
		if _, ok := exportLabels[parts[1]]; !ok {
			return
		}
		b.sendExport(chatID, parts[1])

	case "pg": // pg:<status>:<page> — страница заказов, редактируем сообщение на месте
		if len(parts) < 3 {
			return
		}
		status := parts[1]
		page := int(argAt(2))
		b.renderOrderPage(chatID, cb.Message.MessageID, status, page)

	case "o": // o:<id> — карточка; o:<id>:next — следующий статус; o:<id>:cancel — отмена
		id := argAt(1)
		if len(parts) < 3 {
			o, err := b.repo.GetOrder(id)
			if err != nil {
				b.send(chatID, "Заказ не найден.")
				return
			}
			b.sendKb(chatID, formatOrder(o, false), adminOrderKeyboard(o))
			return
		}
		switch parts[2] {
		case "next":
			o, err := b.repo.GetOrder(id)
			if err != nil {
				b.send(chatID, "Заказ не найден.")
				return
			}
			next := model.NextStatus(o.Status)
			if next == "" {
				b.send(chatID, "Заказ уже в финальном статусе.")
				return
			}
			updated, err := b.svc.TransitionOrder(id, next, cb.From.ID, "")
			if err != nil {
				b.send(chatID, "Ошибка: "+err.Error())
				return
			}
			// Карточку редактируем на месте — чат остаётся чистым.
			b.editOrderCard(chatID, cb.Message.MessageID, updated)
			b.notifyCustomerStatus(id)
		case "cancel":
			b.setWizard(chatID, &wizard{mode: "cancel_reason", orderID: id, msgID: cb.Message.MessageID})
			reply := tgbotapi.NewMessage(chatID, fmt.Sprintf("Причина отмены заказа #%d (коротко):", id))
			reply.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, InputFieldPlaceholder: "например: клиент передумал"}
			if _, err := b.tgSend(reply); err != nil {
				log.Printf("bot send: %v", err)
			}
		}

	case "cl": // cl:<userID> — карточка клиента со статистикой
		b.sendClientDetails(chatID, argAt(1))

	case "flt": // назад к фильтрам — правим то же сообщение
		text, kb := b.buildStatusFilter()
		b.editKb(chatID, cb.Message.MessageID, text, kb)

	case "flt2": // фильтры новым сообщением (из финальной карточки)
		b.sendStatusFilter(chatID)

	case "x": // убрать одно сообщение
		if _, err := b.tgRequest(tgbotapi.NewDeleteMessage(chatID, cb.Message.MessageID)); err != nil {
			log.Printf("bot delete: %v", err)
		}

	case "clean": // очистить историю диалога
		b.cleanChat(chatID)

	case "ophoto": // фото букета → сохранить на заказе и отправить клиенту
		id := argAt(1)
		o, err := b.repo.GetOrder(id)
		if err != nil {
			b.send(chatID, "Заказ не найден.")
			return
		}
		if o.User.TelegramID == 0 {
			b.send(chatID, "У клиента нет Telegram ID (заказ оформлен вне Telegram) — фото отправить некому.")
			return
		}
		b.setWizard(chatID, &wizard{mode: "order_photo", orderID: id, msgID: cb.Message.MessageID})
		kind := "сборки"
		if model.PhotoTypeForStatus(o.Status) == model.PhotoDelivered {
			kind = "вручения"
		}
		ask := tgbotapi.NewMessage(chatID, fmt.Sprintf(
			"📷 Пришлите фото %s букета для заказа #%d — сохраню его на заказе и сразу отправлю клиенту.\n/cancel — отмена.", kind, id))
		ask.ReplyMarkup = tgbotapi.ForceReply{ForceReply: true, InputFieldPlaceholder: "фото букета"}
		if m, err := b.tgSend(ask); err != nil {
			log.Printf("bot send: %v", err)
		} else {
			b.remember(chatID, m.MessageID)
		}

	case "chpub": // подтверждённая публикация поста о товаре в канал
		b.publishProductPost(chatID, argAt(1))

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
			b.sendTemp(chatID, "✅ Изменения сохранены.", 5*time.Second)
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
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
		tgbotapi.NewInlineKeyboardButtonData("🧹 Очистить чат", "clean"),
	))
	b.sendKb(chatID, title, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// --- Админ-CRM: заказы по статусам с пагинацией ---

const ordersPerPage = 5

// sendStatusFilter — /orders: фильтр статусов с бейджами-счётчиками.
func (b *Bot) sendStatusFilter(chatID int64) {
	text, kb := b.buildStatusFilter()
	b.sendKb(chatID, text, kb)
}

func (b *Bot) buildStatusFilter() (string, tgbotapi.InlineKeyboardMarkup) {
	counts, err := b.repo.CountOrdersByStatus()
	if err != nil {
		counts = map[string]int64{}
	}
	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, st := range model.StatusOrder {
		label := fmt.Sprintf("%s (%d)", model.StatusLabels[st], counts[st])
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("pg:%s:1", st)))
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return "Заказы по статусам:", tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// renderOrderPage — редактирует сообщение со списком заказов статуса (5 на страницу).
func (b *Bot) renderOrderPage(chatID int64, msgID int, status string, page int) {
	if page < 1 {
		page = 1
	}
	counts, _ := b.repo.CountOrdersByStatus()
	total := int(counts[status])
	pages := (total + ordersPerPage - 1) / ordersPerPage
	orders, err := b.repo.ListOrdersByStatusPage(status, page, ordersPerPage)
	if err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s — %d шт", model.StatusLabels[status], total)
	if pages > 1 {
		fmt.Fprintf(&sb, " · стр. %d/%d", page, pages)
	}
	sb.WriteString("\n\n")
	var rows [][]tgbotapi.InlineKeyboardButton
	if len(orders) == 0 {
		sb.WriteString("Пусто.")
	}
	var btnRow []tgbotapi.InlineKeyboardButton
	for _, o := range orders {
		pre := ""
		if isPreorder(&o) {
			pre = "📅 "
		}
		fmt.Fprintf(&sb, "%s#%d · %s, %s · %s · %d₽\n",
			pre, o.ID, o.DeliveryDate, o.DeliveryTime, orDash(o.User.Name), o.TotalPrice)
		btnRow = append(btnRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("#%d", o.ID), fmt.Sprintf("o:%d", o.ID)))
		if len(btnRow) == 3 {
			rows = append(rows, btnRow)
			btnRow = nil
		}
	}
	if len(btnRow) > 0 {
		rows = append(rows, btnRow)
	}

	var nav []tgbotapi.InlineKeyboardButton
	if page > 1 {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("← Назад", fmt.Sprintf("pg:%s:%d", status, page-1)))
	}
	nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("Фильтры", "flt"))
	if page < pages {
		nav = append(nav, tgbotapi.NewInlineKeyboardButtonData("Вперёд →", fmt.Sprintf("pg:%s:%d", status, page+1)))
	}
	rows = append(rows, nav)
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
		tgbotapi.NewInlineKeyboardButtonData("🧹 Очистить чат", "clean"),
	))

	b.editKb(chatID, msgID, sb.String(), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// sendPreorders — /preorders: активные заказы на будущие даты.
func (b *Bot) sendPreorders(chatID int64) {
	today := time.Now().Format("2006-01-02")
	orders, err := b.repo.ListPreorders(today, 20)
	if err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	if len(orders) == 0 {
		b.send(chatID, "📅 Предзаказов нет — все заказы на сегодня.")
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "📅 Предзаказы (%d):\n\n", len(orders))
	var rows [][]tgbotapi.InlineKeyboardButton
	var btnRow []tgbotapi.InlineKeyboardButton
	for _, o := range orders {
		fmt.Fprintf(&sb, "#%d · %s, %s · %s · %d₽ · %s\n",
			o.ID, o.DeliveryDate, o.DeliveryTime, orDash(o.User.Name), o.TotalPrice, model.StatusLabels[o.Status])
		btnRow = append(btnRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("#%d", o.ID), fmt.Sprintf("o:%d", o.ID)))
		if len(btnRow) == 3 {
			rows = append(rows, btnRow)
			btnRow = nil
		}
	}
	if len(btnRow) > 0 {
		rows = append(rows, btnRow)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
		tgbotapi.NewInlineKeyboardButtonData("🧹 Очистить чат", "clean"),
	))
	b.sendKb(chatID, sb.String(), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// adminOrderKeyboard — кнопки карточки: следующий шаг конвейера, фото, отмена.
func adminOrderKeyboard(o *model.Order) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	if next := model.NextStatus(o.Status); next != "" {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➡️ "+model.StatusLabels[next], fmt.Sprintf("o:%d:next", o.ID))))
	}
	if o.Status != model.StatusCancelled && o.Status != model.StatusDelivered {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📷 Фото букета", fmt.Sprintf("ophoto:%d", o.ID)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отменить", fmt.Sprintf("o:%d:cancel", o.ID)),
		))
	}
	if len(rows) == 0 {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("К фильтрам", "flt2")))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// editOrderCard — обновляет карточку заказа на месте (editMessageText).
func (b *Bot) editOrderCard(chatID int64, msgID int, o *model.Order) {
	b.editKb(chatID, msgID, formatOrder(o, false), adminOrderKeyboard(o))
}

// NotifyNewOrder шлёт карточку нового заказа всем админам — основной рабочий поток.
// Вызывается из горутины сервиса, поэтому сам отмечается в WaitGroup:
// заказ, принятый до SIGTERM, обязан долететь до админа.
func (b *Bot) NotifyNewOrder(o *model.Order) {
	if !b.trackStart() {
		log.Printf("бот останавливается — уведомление о заказе #%d не отправлено", o.ID)
		return
	}
	defer b.trackDone()
	for _, adminID := range b.adminIDs {
		b.sendKb(adminID, formatOrder(o, true), adminOrderKeyboard(o))
	}
}

// --- Админ-CRM: клиенты ---

// sendClientSearch — /clients <запрос>: до 5 совпадений по имени/телефону.
func (b *Bot) sendClientSearch(chatID int64, query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		b.send(chatID, "Использование: /clients <имя или телефон>\nНапример: /clients мурад или /clients 8999")
		return
	}
	users, err := b.repo.SearchClients(query, 5)
	if err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	if len(users) == 0 {
		b.send(chatID, "Никого не нашлось по запросу «"+query+"».")
		return
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Найдено %d:\n\n", len(users))
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, u := range users {
		fmt.Fprintf(&sb, "• %s · %s\n", orDash(u.Name), orDash(u.Phone))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(
				fmt.Sprintf("Подробнее: %s", orDash(u.Name)), fmt.Sprintf("cl:%d", u.ID))))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
		tgbotapi.NewInlineKeyboardButtonData("🧹 Очистить чат", "clean"),
	))
	b.sendKb(chatID, sb.String(), tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// sendClientDetails — карточка клиента: LTV, средний чек, последние заказы.
func (b *Bot) sendClientDetails(chatID int64, userID uint) {
	u, err := b.repo.GetUserByID(userID)
	if err != nil {
		b.send(chatID, "Клиент не найден.")
		return
	}
	stats, err := b.repo.GetClientStats(userID)
	if err != nil {
		b.send(chatID, "Ошибка: "+err.Error())
		return
	}
	last, _ := b.repo.LastClientOrders(userID, 5)

	var sb strings.Builder
	fmt.Fprintf(&sb, "👤 %s\nТелефон: %s\n\n", orDash(u.Name), orDash(u.Phone))
	fmt.Fprintf(&sb, "Заказов: %d (доставлено %d, отменено %d)\n", stats.Orders, stats.Delivered, stats.Cancelled)
	fmt.Fprintf(&sb, "LTV: %d₽\n", stats.LTV)
	fmt.Fprintf(&sb, "Средний чек: %.0f₽\n", stats.AvgCheck)
	if len(last) > 0 {
		sb.WriteString("\nПоследние заказы:\n")
		var rows [][]tgbotapi.InlineKeyboardButton
		var btnRow []tgbotapi.InlineKeyboardButton
		for _, o := range last {
			fmt.Fprintf(&sb, "#%d · %s · %d₽ · %s\n", o.ID, o.DeliveryDate, o.TotalPrice, model.StatusLabels[o.Status])
			btnRow = append(btnRow, tgbotapi.NewInlineKeyboardButtonData(fmt.Sprintf("#%d", o.ID), fmt.Sprintf("o:%d", o.ID)))
			if len(btnRow) == 3 {
				rows = append(rows, btnRow)
				btnRow = nil
			}
		}
		if len(btnRow) > 0 {
			rows = append(rows, btnRow)
		}
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
			tgbotapi.NewInlineKeyboardButtonData("🧹 Очистить чат", "clean"),
		))
		b.sendKb(chatID, sb.String(), tgbotapi.NewInlineKeyboardMarkup(rows...))
		return
	}
	b.send(chatID, sb.String())
}

func isPreorder(o *model.Order) bool {
	return o.DeliveryDate > time.Now().Format("2006-01-02")
}

func formatOrder(o *model.Order, isNew bool) string {
	var sb strings.Builder
	pre := ""
	if isPreorder(o) {
		pre = "📅 "
	}
	if isNew {
		fmt.Fprintf(&sb, "🌸 %sНовый заказ #%d\n", pre, o.ID)
	} else {
		fmt.Fprintf(&sb, "🌸 %sЗаказ #%d · %s\n", pre, o.ID, model.StatusLabels[o.Status])
	}
	fmt.Fprintf(&sb, "Клиент: %s\n", o.User.Name)
	fmt.Fprintf(&sb, "Телефон: %s\n", o.User.Phone)
	sb.WriteString("Букеты: ")
	for i, it := range o.Items {
		if i == 4 { // не раздуваем карточку: максимум 4 позиции
			fmt.Fprintf(&sb, "; … ещё %d", len(o.Items)-4)
			break
		}
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
	if o.CardText != "" {
		fmt.Fprintf(&sb, "Открытка: %s\n", o.CardText)
	}
	if o.IsAnonymous {
		sb.WriteString("🤫 Анонимная доставка\n")
	}
	if o.Comment != "" {
		fmt.Fprintf(&sb, "Комментарий: %s\n", o.Comment)
	}
	if o.Status == model.StatusCancelled && o.CancelReason != "" {
		fmt.Fprintf(&sb, "Причина отмены: %s\n", o.CancelReason)
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

// saveFresh — сохраняет список свежих цветов на сегодня.
func (b *Bot) saveFresh(chatID int64, items string) {
	items = strings.TrimSpace(items)
	if items == "" {
		b.send(chatID, "Не получилось. Пришлите список цветов.")
		return
	}
	today := time.Now().Format("2006-01-02")
	if err := b.repo.UpsertFreshToday(today, items); err != nil {
		log.Printf("save fresh: %v", err)
		b.send(chatID, "Ошибка сохранения: "+err.Error())
		return
	}
	b.send(chatID, "✅ «Сегодня на базе»: "+items)
}

// handleOrderPhoto — фото букета от админа: сохраняем file_id на заказе
// (тип по текущему статусу: собран/вручён) и мгновенно шлём клиенту с подписью.
func (b *Bot) handleOrderPhoto(chatID int64, w *wizard, fileID string, isDoc bool) {
	o, err := b.repo.GetOrder(w.orderID)
	if err != nil {
		log.Printf("get order: %v", err)
		b.send(chatID, "Ошибка: заказ не найден.")
		return
	}

	photoType := model.PhotoTypeForStatus(o.Status)
	if err := b.repo.AddOrderPhoto(&model.OrderPhoto{
		OrderID: o.ID, Type: photoType, FileID: fileID, IsDocument: isDoc,
	}); err != nil {
		log.Printf("save order photo: %v", err)
		b.send(chatID, "Не удалось сохранить фото: "+err.Error())
		return
	}

	if o.User.TelegramID == 0 {
		b.send(chatID, "Фото сохранено, но у клиента нет Telegram ID — отправить некому.")
		return
	}
	caption, _ := model.PhotoCaption(photoType, o.ID)
	// file_id документа отправляется только через sendDocument (и наоборот) —
	// документ Telegram показывает картинкой с превью, без пересжатия.
	var media tgbotapi.Chattable
	if isDoc {
		doc := tgbotapi.NewDocument(o.User.TelegramID, tgbotapi.FileID(fileID))
		doc.Caption = caption
		media = doc
	} else {
		ph := tgbotapi.NewPhoto(o.User.TelegramID, tgbotapi.FileID(fileID))
		ph.Caption = caption
		media = ph
	}
	if _, err := b.tgSend(media); err != nil {
		log.Printf("send photo to customer: %v", err)
		b.send(chatID, "Фото сохранено, но отправить клиенту не удалось (возможно, он не запускал бота).")
		return
	}

	// Фото сборки двигает конвейер на «Фото отправлено», если это следующий шаг;
	// иначе статус не трогаем — фото просто сохранено и ушло клиенту.
	if photoType == model.PhotoAssembled && model.AllowedTransition(o.Status, model.StatusPhotoSent) {
		updated, err := b.svc.TransitionOrder(o.ID, model.StatusPhotoSent, chatID, "")
		if err != nil {
			log.Printf("update status after photo: %v", err)
		} else if w.msgID != 0 {
			b.editOrderCard(chatID, w.msgID, updated)
		}
		b.send(chatID, fmt.Sprintf("✅ Фото сохранено и отправлено клиенту, заказ #%d → %s.", o.ID, model.StatusLabels[model.StatusPhotoSent]))
		return
	}
	b.send(chatID, fmt.Sprintf("✅ Фото сохранено и отправлено клиенту заказа #%d.", o.ID))
}

// notifyCustomerStatus — отправляет клиенту уведомление о смене статуса заказа.
// Тексты — в model/texts.go; учитываются подарок (открытка/анонимность) и причина отмены.
func (b *Bot) notifyCustomerStatus(orderID uint) {
	o, err := b.repo.GetOrder(orderID)
	if err != nil {
		log.Printf("get order for status notification: %v", err)
		return
	}
	text, ok := model.ClientOrderStatusText(o)
	if !ok {
		return
	}
	if o.User.TelegramID == 0 {
		return // Нет способа отправить сообщение, если клиент не сохранён.
	}
	// Клиентский чат не админский — remember() его не логирует, /clean не тронет.
	b.send(o.User.TelegramID, text)
}
