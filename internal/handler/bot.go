package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/config"
	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

// Таймауты и лимиты работы с Telegram.
const (
	telegramTimeout = 30 * time.Second
	// updateTimeout — потолок на обработку одного апдейта (включая SQL).
	updateTimeout = 25 * time.Second
	// wizardTTL — заброшенный визард не должен вечно перехватывать текст админа.
	wizardTTL = 30 * time.Minute
	msgLogCap = 400
	// maxPhotosPerProduct — столько фото помещается в карточку без утомления.
	maxPhotosPerProduct = 5
)

type Bot struct {
	api   *tgbotapi.BotAPI
	repo  *repository.Repository
	svc   *service.Service
	store storage.Storage
	cfg   *config.Config
	log   *slog.Logger

	// Состояние визардов /add и /edit. Конкурентная запись в map в Go —
	// фатальная ошибка, а не паника, перехватить её нельзя.
	wizMu   sync.Mutex
	wizards map[int64]*wizard

	// chats сериализует обработку апдейтов одного чата: черновик визарда
	// мутируется вне wizMu, а в режиме webhook апдейты идут параллельно.
	chats *chatLocks

	// Журнал сообщений в админ-чатах для /clean.
	logMu  sync.Mutex
	msgLog map[int64][]int

	// wg считает апдейты, обрабатываемые в фоне (режим webhook), чтобы
	// graceful shutdown дождался их, а не оборвал на полуслове.
	wg       sync.WaitGroup
	stopOnce sync.Once
	done     chan struct{}
	// polling выставляется до запуска горутин и дальше не меняется —
	// иначе Stop() читал бы поле, которое пишет Run() в другой горутине.
	polling bool

	// welcomePhoto — снимок первого экрана, разобранный один раз на старте.
	welcomePhoto string
}

type wizard struct {
	mode      string // add | edit_text | edit_variants | edit_photos | edit_discount | edit_stock | fresh | order_photo | cancel_reason | promo_add
	step      string // для add: name → photos → desc → variants → category → confirm
	productID uint
	orderID   uint
	msgID     int    // карточка, которую правим после действия
	field     string // name | description
	draft     draft
	promo     promoDraft
	touched   time.Time
}

// promoDraft — накопленные шаги визарда /promoadd.
type promoDraft struct {
	code          string
	discountType  string
	discountValue int
	minOrder      int
	expiresAt     *time.Time
}

type draft struct {
	name        string
	description string
	category    string
	variants    []model.ProductVariant
	imageURLs   []string
}

// NewBot создаёт бота. Токен читается вызывающим кодом из окружения —
// в самом пакете его нет и быть не может.
func NewBot(cfg *config.Config, log *slog.Logger, repo *repository.Repository, svc *service.Service, store storage.Storage) (*Bot, error) {
	// Собственный http-клиент: у клиента по умолчанию нет таймаута,
	// и зависшее соединение с Telegram держало бы горутину вечно.
	client := &http.Client{Timeout: telegramTimeout}
	api, err := tgbotapi.NewBotAPIWithClient(cfg.BotToken, tgbotapi.APIEndpoint, client)
	if err != nil {
		return nil, fmt.Errorf("подключение к Telegram: %w", err)
	}

	b := &Bot{
		api:     api,
		repo:    repo,
		svc:     svc,
		store:   store,
		cfg:     cfg,
		log:     log.With("component", "bot", "bot_username", api.Self.UserName),
		wizards: map[int64]*wizard{},
		chats:   newChatLocks(),
		msgLog:  map[int64][]int{},
		done:    make(chan struct{}),
		polling: cfg.BotMode == "polling",

		welcomePhoto: resolveWelcomePhoto(cfg),
	}
	if b.welcomePhoto == "" {
		b.log.Warn("приветственное фото не задано — первый экран уходит текстом",
			"подсказка", "положите web/public/"+welcomeAsset+" или задайте WELCOME_PHOTO")
	}
	svc.NotifyNewOrder = b.NotifyNewOrder

	// Настройка бота в Telegram — в фоне: это два сетевых вызова, и если
	// Telegram отвечает медленно, HTTP-сервер не должен ждать их старта,
	// иначе healthcheck Railway успевает провалиться.
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				b.log.Error("паника при настройке бота", "panic", rec)
			}
		}()
		b.syncMenuButton()
		b.syncCommands()
	}()
	return b, nil
}

// Username — @имя текущего бота (для логов и deep-link-ов).
func (b *Bot) Username() string { return b.api.Self.UserName }

func (b *Bot) isAdmin(id int64) bool {
	for _, a := range b.cfg.AdminIDs {
		if a == id {
			return true
		}
	}
	return false
}

// ─── Визарды: доступ под мьютексом ─────────────────────────────────────────

func (b *Bot) getWizard(chatID int64) (*wizard, bool) {
	b.wizMu.Lock()
	defer b.wizMu.Unlock()
	w, ok := b.wizards[chatID]
	if !ok {
		return nil, false
	}
	if time.Since(w.touched) > wizardTTL {
		delete(b.wizards, chatID)
		return nil, false
	}
	w.touched = time.Now()
	return w, true
}

func (b *Bot) setWizard(chatID int64, w *wizard) {
	w.touched = time.Now()
	b.wizMu.Lock()
	b.wizards[chatID] = w
	b.wizMu.Unlock()
}

func (b *Bot) clearWizard(chatID int64) {
	b.wizMu.Lock()
	delete(b.wizards, chatID)
	b.wizMu.Unlock()
}

// ─── Запуск и остановка ────────────────────────────────────────────────────

// Run — long polling. Возвращается, когда вызван Stop.
func (b *Bot) Run() {
	defer close(b.done)
	b.log.Info("бот запущен в режиме long polling")
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	u.AllowedUpdates = []string{"message", "callback_query"}
	for update := range b.api.GetUpdatesChan(u) {
		b.dispatch(update)
	}
	b.log.Info("приём апдейтов остановлен")
}

// Stop прекращает приём апдейтов и ждёт, пока доработают начатые обработчики.
func (b *Bot) Stop(ctx context.Context) {
	b.stopOnce.Do(func() {
		if b.polling {
			b.api.StopReceivingUpdates()
			select {
			case <-b.done:
			case <-ctx.Done():
			}
		}
		// Апдейты из webhook обрабатываются в фоновых горутинах — дожидаемся их.
		waited := make(chan struct{})
		go func() {
			b.wg.Wait()
			close(waited)
		}()
		select {
		case <-waited:
		case <-ctx.Done():
			b.log.Warn("часть апдейтов не успела обработаться до остановки")
		}
	})
}

// dispatch — единая точка обработки апдейта: изоляция паники, таймаут,
// корреляция логов по update_id.
func (b *Bot) dispatch(update tgbotapi.Update) {
	log := b.log.With("update_id", update.UpdateID)
	defer func() {
		if rec := recover(); rec != nil {
			log.Error("паника при обработке апдейта", "panic", rec)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), updateTimeout)
	defer cancel()

	// Апдейты одного чата обрабатываем строго по очереди — см. chatLocks.
	if chatID := updateChatID(update); chatID != 0 {
		defer b.chats.Lock(chatID)()
	}

	switch {
	case update.CallbackQuery != nil:
		b.handleCallback(ctx, log, update.CallbackQuery)
	case update.Message != nil:
		b.handleMessage(ctx, log, update.Message)
	}
}

// updateChatID — чат, к которому относится апдейт (0, если определить нельзя).
func updateChatID(u tgbotapi.Update) int64 {
	switch {
	case u.CallbackQuery != nil && u.CallbackQuery.Message != nil && u.CallbackQuery.Message.Chat != nil:
		return u.CallbackQuery.Message.Chat.ID
	case u.Message != nil && u.Message.Chat != nil:
		return u.Message.Chat.ID
	}
	return 0
}

// ─── Отправка с ретраями ───────────────────────────────────────────────────

// sendRetry выполняет запрос к Telegram с ограниченным числом повторов.
// Повторяем только то, что имеет смысл повторять: сетевые сбои, 429 и 5xx.
// Ошибки вида «бот заблокирован» повторять бессмысленно.
func (b *Bot) sendRetry(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		msg, err := b.api.Send(c)
		if err == nil {
			return msg, nil
		}
		lastErr = err

		var tgErr *tgbotapi.Error
		if errors.As(err, &tgErr) {
			if tgErr.RetryAfter > 0 {
				wait := time.Duration(tgErr.RetryAfter) * time.Second
				if wait > 30*time.Second {
					return msg, err // ждать дольше получаса смысла нет
				}
				time.Sleep(wait)
				continue
			}
			if tgErr.Code >= 400 && tgErr.Code < 500 {
				return msg, err // 400/403 повторять бесполезно
			}
		}
		if i < attempts-1 {
			time.Sleep(time.Duration(1<<i) * 500 * time.Millisecond)
		}
	}
	return tgbotapi.Message{}, lastErr
}

func (b *Bot) request(c tgbotapi.Chattable) error {
	_, err := b.api.Request(c)
	return err
}

// send — обычное сообщение. Ошибка логируется, но никогда не всплывает:
// сбой уведомления не должен ломать основной поток.
func (b *Bot) send(chatID int64, text string) {
	b.sendKb(chatID, text, nil)
}

func (b *Bot) sendKb(chatID int64, text string, kb *tgbotapi.InlineKeyboardMarkup) {
	msg := tgbotapi.NewMessage(chatID, clipTelegram(text))
	if kb != nil {
		msg.ReplyMarkup = *kb
	}
	m, err := b.sendRetry(msg)
	if err != nil {
		b.log.Error("не удалось отправить сообщение", "chat_id", chatID, "err", err)
		return
	}
	b.remember(chatID, m.MessageID)
}

// sendTemp — служебное сообщение, которое самоуничтожается через ttl.
// Таймер не переживает рестарт, поэтому сообщение попадает и в журнал /clean.
func (b *Bot) sendTemp(chatID int64, text string, ttl time.Duration) {
	m, err := b.sendRetry(tgbotapi.NewMessage(chatID, clipTelegram(text)))
	if err != nil {
		b.log.Error("не удалось отправить сообщение", "chat_id", chatID, "err", err)
		return
	}
	b.remember(chatID, m.MessageID)
	time.AfterFunc(ttl, func() {
		_ = b.request(tgbotapi.NewDeleteMessage(chatID, m.MessageID))
	})
}

// editKb правит текст и клавиатуру существующего сообщения (чистый чат вместо спама).
func (b *Bot) editKb(chatID int64, msgID int, text string, kb tgbotapi.InlineKeyboardMarkup) {
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, clipTelegram(text), kb)
	if _, err := b.sendRetry(edit); err != nil {
		// «message is not modified» — не ошибка, а повторный тап по той же кнопке.
		if !strings.Contains(err.Error(), "message is not modified") {
			b.log.Warn("не удалось обновить сообщение", "chat_id", chatID, "message_id", msgID, "err", err)
		}
	}
}

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

// cleanChat — /clean: удаляет запомненные сообщения диалога.
// Telegram позволяет удалять сообщения младше 48 часов; старые пропускаем.
func (b *Bot) cleanChat(chatID int64) {
	b.logMu.Lock()
	ids := b.msgLog[chatID]
	delete(b.msgLog, chatID)
	b.logMu.Unlock()

	deleted := 0
	for _, id := range ids {
		if err := b.request(tgbotapi.NewDeleteMessage(chatID, id)); err == nil {
			deleted++
		}
	}
	b.sendTemp(chatID, fmt.Sprintf("🧹 Убрано %d сообщений.", deleted), 4*time.Second)
}

// clipTelegram обрезает текст под лимит сообщения Telegram (4096 символов).
func clipTelegram(s string) string {
	const limit = 4000
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return string(r[:limit]) + "\n…"
}

// ─── Настройка бота ────────────────────────────────────────────────────────

// BotCommands — единый источник правды для /setcommands в BotFather
// и для меню команд, которое бот выставляет себе сам при старте.
var BotCommands = []struct{ Command, Description string }{
	{"start", "Открыть каталог"},
	{"orders", "Заказы по статусам"},
	{"today", "Доставки на сегодня"},
	{"preorders", "Предзаказы на будущие даты"},
	{"clients", "Найти клиента по имени или телефону"},
	{"add", "Добавить товар"},
	{"edit", "Изменить товар"},
	{"hide", "Скрыть или показать товар"},
	{"delete", "Удалить товар"},
	{"promo", "Промокоды: список и вкл/выкл"},
	{"promoadd", "Создать промокод"},
	{"fresh", "Что сегодня свежее на базе"},
	{"clean", "Очистить историю чата"},
	{"cancel", "Прервать текущее действие"},
	{"help", "Список команд"},
}

// syncCommands выставляет меню команд бота, чтобы новый бот сразу был рабочим
// даже до того, как владелец дойдёт до /setcommands в BotFather.
func (b *Bot) syncCommands() {
	cmds := make([]tgbotapi.BotCommand, 0, len(BotCommands))
	for _, c := range BotCommands {
		cmds = append(cmds, tgbotapi.BotCommand{Command: c.Command, Description: c.Description})
	}
	if err := b.request(tgbotapi.NewSetMyCommands(cmds...)); err != nil {
		b.log.Warn("не удалось выставить меню команд", "err", err)
	}
}

// syncMenuButton делает кнопку меню бота постоянной ссылкой на Mini App.
// Без неё магазин открывается только из inline-кнопки конкретного сообщения,
// а в старых сообщениях адрес вшит навсегда — после переезда они ведут в никуда.
//
// Метод появился в Bot API 6.0, в tgbotapi v5.5.1 его нет — зовём напрямую.
func (b *Bot) syncMenuButton() {
	if b.cfg.PublicURL == "" {
		b.log.Warn("PUBLIC_URL не задан — кнопка меню магазина не настроена")
		return
	}
	body, err := json.Marshal(map[string]any{
		"menu_button": map[string]any{
			"type":    "web_app",
			"text":    "🌸 Каталог",
			"web_app": map[string]string{"url": b.cfg.PublicURL},
		},
	})
	if err != nil {
		return
	}
	if err := b.rawAPI("setChatMenuButton", body); err != nil {
		b.log.Warn("не удалось настроить кнопку меню", "err", err)
		return
	}
	b.log.Info("кнопка меню бота ведёт на Mini App", "url", b.cfg.PublicURL)
}

// rawAPI вызывает метод Bot API, которого нет в tgbotapi v5.5.1.
// Токен в URL — поэтому текст ошибки никогда не логируется целиком.
func (b *Bot) rawAPI(method string, body []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), telegramTimeout)
	defer cancel()

	endpoint := fmt.Sprintf(tgbotapi.APIEndpoint, b.api.Token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: telegramTimeout}).Do(req)
	if err != nil {
		// Сообщение об ошибке содержит URL с токеном — наружу его не отдаём.
		return fmt.Errorf("telegram %s: сетевая ошибка", method)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram %s: HTTP %d", method, resp.StatusCode)
	}
	return nil
}

// ─── Маршрутизация сообщений ───────────────────────────────────────────────

func (b *Bot) handleMessage(ctx context.Context, log *slog.Logger, msg *tgbotapi.Message) {
	if msg.From == nil || msg.Chat == nil {
		return
	}
	// Права проверяем по автору сообщения, а не по чату: в группе chat.ID
	// не равен user.ID, и проверка по чату пускала бы посторонних.
	if !b.isAdmin(msg.From.ID) || !b.isAdmin(msg.Chat.ID) {
		b.handleCustomer(ctx, log, msg)
		return
	}
	log = log.With("admin_id", msg.From.ID)

	// Помним и входящие сообщения админа — /clean уберёт и их.
	b.remember(msg.Chat.ID, msg.MessageID)

	if msg.IsCommand() {
		b.handleAdminCommand(ctx, log, msg)
		return
	}
	if w, ok := b.getWizard(msg.Chat.ID); ok {
		b.wizardInput(ctx, log, msg, w)
		return
	}
	b.send(msg.Chat.ID, "Не понял. /help — список команд.")
}

func (b *Bot) handleAdminCommand(ctx context.Context, log *slog.Logger, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	switch msg.Command() {
	case "start", "help":
		b.send(chatID, adminHelp())
	case "orders":
		b.sendStatusFilter(ctx, chatID)
	case "today":
		b.sendToday(ctx, chatID)
	case "preorders":
		b.sendPreorders(ctx, chatID)
	case "clients":
		b.sendClientSearch(ctx, chatID, msg.CommandArguments())
	case "add":
		b.setWizard(chatID, &wizard{mode: "add", step: "name"})
		b.send(chatID, "🌸 Новый товар.\n\nШаг 1/5 — введите название:")
	case "edit":
		b.sendProductList(ctx, chatID, "Что редактируем?", "edit")
	case "hide":
		b.sendProductList(ctx, chatID, "Какой товар скрыть/показать?", "hide")
	case "delete":
		b.sendProductList(ctx, chatID, "Какой товар удалить?", "del")
	case "promo":
		b.sendPromoList(ctx, chatID)
	case "promoadd":
		b.startPromoWizard(chatID)
	case "fresh":
		if items := strings.TrimSpace(msg.CommandArguments()); items != "" {
			b.saveFresh(ctx, chatID, items)
			return
		}
		b.setWizard(chatID, &wizard{mode: "fresh"})
		b.send(chatID, "🌷 Что сегодня свежее? Пришлите одной строкой, например:\nпионы, ранункулюсы, эустома")
	case "done":
		b.wizardDone(ctx, chatID)
	case "clean":
		b.cleanChat(chatID)
	case "cancel":
		b.clearWizard(chatID)
		b.sendTemp(chatID, "Действие отменено.", 4*time.Second)
	default:
		log.Info("неизвестная команда", "command", msg.Command())
		b.send(chatID, "Неизвестная команда. /help — список команд.")
	}
}

func adminHelp() string {
	var sb strings.Builder
	sb.WriteString("Команды администратора:\n")
	for _, c := range BotCommands {
		if c.Command == "start" {
			continue
		}
		fmt.Fprintf(&sb, "/%s — %s\n", c.Command, strings.ToLower(c.Description))
	}
	return strings.TrimSpace(sb.String())
}

// handleCustomer — не-админ: первый экран и промокод из deep-link.
// Ответ всегда один и тот же по форме — приветствие и одна кнопка в каталог:
// любая развилка на этом шаге стоит клиентов.
func (b *Bot) handleCustomer(ctx context.Context, log *slog.Logger, msg *tgbotapi.Message) {
	name := msg.From.FirstName

	if msg.IsCommand() && msg.Command() == "start" {
		if code := strings.TrimSpace(msg.CommandArguments()); code != "" {
			promo, err := b.svc.ApplyDeepLinkPromo(ctx, msg.From.ID, code)
			if err == nil {
				b.sendWelcome(msg.Chat.ID, promoWelcomeText(
					name, promo.Code, promo.Describe(), promoMinimumHint(promo)))
				return
			}
			var ve *service.ValidationError
			if !errors.As(err, &ve) {
				log.Error("не удалось применить промокод из deep-link", "err", err)
			}
			// Промокод не сработал — про это ни слова: испорченная ссылка не повод
			// начинать знакомство с извинений. Показываем обычное приветствие.
		}
	}
	b.sendWelcome(msg.Chat.ID, welcomeText(name))
}

// webAppKeyboard — inline-кнопка web_app (запуск Mini App). В tgbotapi v5.5.1
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

// ─── Маршрутизация callback-кнопок ─────────────────────────────────────────

func (b *Bot) handleCallback(ctx context.Context, log *slog.Logger, cb *tgbotapi.CallbackQuery) {
	// Ответить на callback обязаны всегда, иначе у админа крутится часик.
	defer func() {
		if err := b.request(tgbotapi.NewCallback(cb.ID, "")); err != nil {
			log.Debug("не удалось подтвердить callback", "err", err)
		}
	}()

	// Message может отсутствовать (например, у слишком старых сообщений).
	if cb.Message == nil || cb.Message.Chat == nil || cb.From == nil {
		return
	}
	chatID := cb.Message.Chat.ID
	// Payload кнопки можно подделать — права проверяем и у чата, и у автора.
	if !b.isAdmin(chatID) || !b.isAdmin(cb.From.ID) {
		log.Warn("callback от не-админа отклонён", "from_id", cb.From.ID, "chat_id", chatID)
		return
	}
	log = log.With("admin_id", cb.From.ID)

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
	case "cat", "addok", "addcancel", "edit", "editf", "hide", "hideok", "del", "delok":
		b.handleCatalogCallback(ctx, log, cb, action, parts, argAt)
	case "pg", "o", "cl", "flt", "flt2", "ophoto":
		b.handleOrderCallback(ctx, log, cb, action, parts, argAt)
	case "pr":
		b.handlePromoCallback(ctx, log, cb, parts, argAt)
	case "x":
		if err := b.request(tgbotapi.NewDeleteMessage(chatID, cb.Message.MessageID)); err != nil {
			log.Debug("не удалось удалить сообщение", "err", err)
		}
	case "clean":
		b.cleanChat(chatID)
	case "noop":
		// отмена подтверждения — ничего не делаем
	default:
		if strings.HasPrefix(action, "editcat_") {
			b.handleCatalogCallback(ctx, log, cb, action, parts, argAt)
		}
	}
}

// adminError показывает админу понятную причину и оставляет подробности в логе.
// Сырой текст ошибки БД в чат не уходит: он может содержать данные подключения.
func (b *Bot) adminError(log *slog.Logger, chatID int64, op string, err error) {
	var ve *service.ValidationError
	if errors.As(err, &ve) {
		b.send(chatID, "⚠️ "+ve.Msg)
		return
	}
	if errors.Is(err, repository.ErrNotFound) {
		b.send(chatID, "⚠️ Запись не найдена — возможно, её уже удалили.")
		return
	}
	log.Error("ошибка админ-действия", "op", op, "err", err)
	b.send(chatID, "⚠️ Не получилось выполнить действие. Попробуйте ещё раз через минуту.")
}
