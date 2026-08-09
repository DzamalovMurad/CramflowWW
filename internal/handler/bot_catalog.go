package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/storage"
)

// Границы полей товара — те же принципы, что и для клиентского ввода.
const (
	maxProductNameLen = 80
	maxProductDescLen = 800
	maxVariantsPerAdd = 8
	maxVariantPrice   = 1_000_000
	maxVariantQty     = 999
)

// sendAsFileHint — единая подсказка, как прислать фото без потери качества.
const sendAsFileHint = "Пришлите фото файлом, без сжатия:\n" +
	"📎 → Файл (не «Фото») → выберите снимок.\n" +
	"На iPhone: 📎 → Файл; если выбираете из галереи — сначала «Сохранить в Файлы».\n" +
	"Так на витрину попадёт оригинал, а не сжатая Telegram копия."

var variantRe = regexp.MustCompile(`(\d+)\s*шт\s*[—–-]+\s*(\d+)`)

// parseVariants разбирает строку вида «9 шт — 2990₽; 15 шт — 4490₽».
func parseVariants(s string) ([]model.ProductVariant, error) {
	matches := variantRe.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("не понял формат")
	}
	if len(matches) > maxVariantsPerAdd {
		return nil, fmt.Errorf("слишком много вариантов, максимум %d", maxVariantsPerAdd)
	}
	seen := map[int]bool{}
	out := make([]model.ProductVariant, 0, len(matches))
	for _, m := range matches {
		qty, err1 := strconv.Atoi(m[1])
		price, err2 := strconv.Atoi(m[2])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("не понял числа в «%s»", m[0])
		}
		if qty < 1 || qty > maxVariantQty {
			return nil, fmt.Errorf("количество цветов должно быть от 1 до %d", maxVariantQty)
		}
		if price < 1 || price > maxVariantPrice {
			return nil, fmt.Errorf("цена должна быть от 1 до %d ₽", maxVariantPrice)
		}
		if seen[qty] {
			return nil, fmt.Errorf("вариант «%d шт» указан дважды", qty)
		}
		seen[qty] = true
		out = append(out, model.ProductVariant{Quantity: qty, Price: price})
	}
	return out, nil
}

// ─── Ввод в визардах ───────────────────────────────────────────────────────

func (b *Bot) wizardInput(ctx context.Context, log *slog.Logger, msg *tgbotapi.Message, w *wizard) {
	chatID := msg.Chat.ID

	// Сжатое фото не принимаем: Telegram ужимает его до ~1280px, на витрине это мыло.
	if len(msg.Photo) > 0 {
		b.send(chatID, sendAsFileHint)
		return
	}

	if msg.Document != nil {
		b.wizardDocument(ctx, log, msg, w)
		return
	}

	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}
	if utf8.RuneCountInString(text) > 4000 {
		b.send(chatID, "Слишком длинный текст.")
		return
	}

	switch w.mode {
	case "add":
		b.wizardAddText(chatID, w, text)
	case "fresh":
		b.clearWizard(chatID)
		b.saveFresh(ctx, chatID, text)
	case "cancel_reason":
		b.finishCancel(ctx, log, msg, w, text)
	case "order_photo":
		b.send(chatID, "Жду фото букета файлом. Или /cancel — отмена.")
	case "edit_text":
		b.editProductText(ctx, log, chatID, w, text)
	case "edit_variants":
		variants, err := parseVariants(text)
		if err != nil {
			b.send(chatID, fmt.Sprintf("%s. Пример: 9 шт — 2990₽; 15 шт — 4490₽", capitalize(err.Error())))
			return
		}
		if err := b.repo.ReplaceVariants(ctx, w.productID, variants); err != nil {
			b.adminError(log, chatID, "replace variants", err)
			return
		}
		b.clearWizard(chatID)
		b.sendTemp(chatID, "✅ Цены обновлены.", 5*time.Second)
	case "edit_discount":
		b.editDiscount(ctx, log, chatID, w, text)
	case "edit_stock":
		b.editStock(ctx, log, chatID, w, text)
	case "edit_photos":
		b.send(chatID, "Отправьте фото файлом (до 5 шт) или /done — завершить.")
	case "promo_add":
		b.promoWizardInput(ctx, log, chatID, w, text)
	}
}

// wizardDocument принимает присланный файлом снимок.
func (b *Bot) wizardDocument(ctx context.Context, log *slog.Logger, msg *tgbotapi.Message, w *wizard) {
	chatID := msg.Chat.ID
	doc := msg.Document

	if !isImageDocument(doc) {
		b.send(chatID, "Это не изображение. "+sendAsFileHint)
		return
	}

	// Фото готового букета: пересылаем клиенту по file_id, без скачивания.
	// file_id принадлежит этому же боту и получен только что — он валиден.
	if w.mode == "order_photo" {
		b.clearWizard(chatID)
		b.sendBouquetPhoto(ctx, log, chatID, w.orderID, doc.FileID)
		return
	}

	if (w.mode != "add" || w.step != "photos") && w.mode != "edit_photos" {
		b.send(chatID, "Фото сейчас не ожидается.")
		return
	}
	if len(w.draft.imageURLs) >= maxPhotosPerProduct {
		b.send(chatID, "Уже 5 фото — больше не нужно. /done — завершить.")
		return
	}

	url, err := b.saveDocument(ctx, doc)
	if err != nil {
		log.Error("не удалось сохранить фото товара", "err", err)
		b.send(chatID, "Не удалось сохранить фото. Попробуйте ещё раз или пришлите другой файл.")
		return
	}
	w.draft.imageURLs = append(w.draft.imageURLs, url)
	n := len(w.draft.imageURLs)
	if n >= maxPhotosPerProduct {
		b.wizardDone(ctx, chatID)
		return
	}
	b.send(chatID, fmt.Sprintf("🖼 Фото %d/%d сохранено в оригинале. Отправьте ещё или /done.", n, maxPhotosPerProduct))
}

func (b *Bot) wizardAddText(chatID int64, w *wizard, text string) {
	switch w.step {
	case "name":
		if utf8.RuneCountInString(text) > maxProductNameLen {
			b.send(chatID, fmt.Sprintf("Название — не более %d символов.", maxProductNameLen))
			return
		}
		w.draft.name = text
		w.step = "photos"
		b.send(chatID, "Шаг 2/5 — отправьте 4–5 фото букета по одному.\n\n"+sendAsFileHint+"\n\nКогда закончите — /done.")
	case "photos":
		b.send(chatID, "Жду фото файлом. Когда закончите — /done.")
	case "desc":
		if utf8.RuneCountInString(text) > maxProductDescLen {
			b.send(chatID, fmt.Sprintf("Описание — не более %d символов.", maxProductDescLen))
			return
		}
		if text != "-" {
			w.draft.description = text
		}
		w.step = "variants"
		b.send(chatID, "Шаг 4/5 — варианты и цены одной строкой.\nПример: 9 шт — 2990₽; 15 шт — 4490₽")
	case "variants":
		variants, err := parseVariants(text)
		if err != nil {
			b.send(chatID, fmt.Sprintf("%s. Пример: 9 шт — 2990₽; 15 шт — 4490₽", capitalize(err.Error())))
			return
		}
		w.draft.variants = variants
		w.step = "category"
		kb := categoryKeyboard("cat")
		b.sendKb(chatID, "Шаг 5/5 — выберите категорию:", &kb)
	}
}

// wizardDone — /done: завершение приёма фото.
func (b *Bot) wizardDone(ctx context.Context, chatID int64) {
	w, ok := b.getWizard(chatID)
	if !ok {
		b.send(chatID, "Сейчас нечего завершать.")
		return
	}
	log := b.log
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
		if err := b.repo.ReplaceImages(ctx, w.productID, w.draft.imageURLs); err != nil {
			b.adminError(log, chatID, "replace images", err)
			return
		}
		b.clearWizard(chatID)
		b.sendTemp(chatID, "✅ Фото обновлены.", 5*time.Second)
	default:
		b.send(chatID, "Сейчас нечего завершать.")
	}
}

func (b *Bot) editProductText(ctx context.Context, log *slog.Logger, chatID int64, w *wizard, text string) {
	fields := map[string]any{}
	if w.field == "name" {
		if utf8.RuneCountInString(text) > maxProductNameLen {
			b.send(chatID, fmt.Sprintf("Название — не более %d символов.", maxProductNameLen))
			return
		}
		fields["name"] = text
	} else {
		if utf8.RuneCountInString(text) > maxProductDescLen {
			b.send(chatID, fmt.Sprintf("Описание — не более %d символов.", maxProductDescLen))
			return
		}
		if text == "-" {
			text = ""
		}
		fields["description"] = text
	}
	if err := b.repo.UpdateProductFields(ctx, w.productID, fields); err != nil {
		b.adminError(log, chatID, "update product", err)
		return
	}
	b.clearWizard(chatID)
	b.sendTemp(chatID, "✅ Изменения сохранены.", 5*time.Second)
}

func (b *Bot) editDiscount(ctx context.Context, log *slog.Logger, chatID int64, w *wizard, text string) {
	pct, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || pct < 0 || pct > 90 {
		b.send(chatID, "Нужно число от 0 до 90. Попробуйте ещё раз или /cancel.")
		return
	}
	if err := b.repo.SetProductDiscount(ctx, w.productID, pct); err != nil {
		b.adminError(log, chatID, "set discount", err)
		return
	}
	b.clearWizard(chatID)
	if pct == 0 {
		b.sendTemp(chatID, "✅ Скидка убрана.", 5*time.Second)
		return
	}
	b.send(chatID, fmt.Sprintf("✅ Скидка −%d%% включена, на витрине появится бейдж.", pct))
}

func (b *Bot) editStock(ctx context.Context, log *slog.Logger, chatID int64, w *wizard, text string) {
	raw := strings.TrimSpace(text)
	// «-» — снять учёт остатка; число — включить учёт с этим количеством.
	if raw == "-" {
		if err := b.repo.UpdateProductFields(ctx, w.productID, map[string]any{"stock": nil}); err != nil {
			b.adminError(log, chatID, "set stock", err)
			return
		}
		b.clearWizard(chatID)
		b.sendTemp(chatID, "✅ Учёт остатка снят — количество не ограничено.", 5*time.Second)
		return
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > 9999 {
		b.send(chatID, "Нужно число от 0 до 9999 или «-», чтобы не ограничивать. Или /cancel.")
		return
	}
	if err := b.repo.UpdateProductFields(ctx, w.productID, map[string]any{"stock": n}); err != nil {
		b.adminError(log, chatID, "set stock", err)
		return
	}
	b.clearWizard(chatID)
	if n == 0 {
		b.send(chatID, "✅ Остаток 0 — букет помечен как «нет в наличии» и заказать его нельзя.")
		return
	}
	b.send(chatID, fmt.Sprintf("✅ Остаток %d шт. Больше этого количества клиент не закажет, "+
		"при ≤5 на витрине появится бейдж «осталось N», а при отмене заказа остаток вернётся.", n))
}

// ─── Фото ──────────────────────────────────────────────────────────────────

func isImageDocument(doc *tgbotapi.Document) bool {
	if doc == nil {
		return false
	}
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

// saveDocument скачивает присланный файлом снимок и кладёт в хранилище.
func (b *Bot) saveDocument(ctx context.Context, doc *tgbotapi.Document) (string, error) {
	if doc.FileSize > storage.MaxUploadBytes {
		return "", fmt.Errorf("файл больше %d МБ — Telegram не отдаёт такие ботам", storage.MaxUploadBytes>>20)
	}
	fileURL, err := b.api.GetFileDirectURL(doc.FileID)
	if err != nil {
		return "", fmt.Errorf("получение ссылки на файл: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return "", fmt.Errorf("подготовка запроса файла: %w", err)
	}
	resp, err := b.api.Client.Do(req)
	if err != nil {
		// URL содержит токен бота — наружу его не отдаём.
		return "", fmt.Errorf("скачивание файла из Telegram не удалось")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Telegram вернул %d при скачивании", resp.StatusCode)
	}

	name := doc.FileName
	if name == "" {
		name = "photo.jpg"
	}
	return b.store.Save(ctx, name, resp.Body)
}

// ─── Списки и клавиатуры ───────────────────────────────────────────────────

func categoryKeyboard(prefix string) tgbotapi.InlineKeyboardMarkup {
	emoji := []string{"💚", "💎", "✨", "⚡"}
	var rows [][]tgbotapi.InlineKeyboardButton
	for i, c := range model.Categories {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(emoji[i]+" "+c, fmt.Sprintf("%s:%d", prefix, i))))
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// closeRow — единый «подвал» списков: закрыть сообщение или почистить чат.
func closeRow() []tgbotapi.InlineKeyboardButton {
	return tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"),
		tgbotapi.NewInlineKeyboardButtonData("🧹 Очистить чат", "clean"),
	)
}

func (b *Bot) sendProductList(ctx context.Context, chatID int64, title, action string) {
	products, err := b.repo.ListAllProducts(ctx)
	if err != nil {
		b.adminError(b.log, chatID, "list products", err)
		return
	}
	if len(products) == 0 {
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
			tgbotapi.NewInlineKeyboardButtonData(clipLabel(label), fmt.Sprintf("%s:%d", action, p.ID))))
		if len(rows) >= 20 {
			break
		}
	}
	rows = append(rows, closeRow())
	kb := tgbotapi.NewInlineKeyboardMarkup(rows...)
	b.sendKb(chatID, title, &kb)
}

// ─── Callback-и каталога ───────────────────────────────────────────────────

func (b *Bot) handleCatalogCallback(ctx context.Context, log *slog.Logger, cb *tgbotapi.CallbackQuery, action string, parts []string, argAt func(int) uint) {
	chatID := cb.Message.Chat.ID

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
		b.sendKb(chatID, summary, &kb)

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
		if err := b.repo.CreateProduct(ctx, p); err != nil {
			b.adminError(log, chatID, "create product", err)
			return
		}
		b.clearWizard(chatID)
		b.send(chatID, fmt.Sprintf("✅ Товар «%s» добавлен (#%d) и уже виден в магазине.", p.Name, p.ID))

	case "addcancel":
		b.clearWizard(chatID)
		b.sendTemp(chatID, "Добавление отменено.", 5*time.Second)

	case "edit":
		id := argAt(1)
		p, err := b.repo.GetProduct(ctx, id)
		if err != nil {
			b.adminError(log, chatID, "get product", err)
			return
		}
		hitLabel := "⭐ Хит: выкл"
		if p.IsHit {
			hitLabel = "⭐ Хит: вкл"
		}
		visLabel := "🙈 Скрыть"
		if p.IsHidden {
			visLabel = "👁 Показать"
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
			// Скрыть/показать прямо отсюда: раньше ради этого приходилось
			// выходить в /hide и заново искать товар в списке.
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(visLabel, fmt.Sprintf("hideok:%d", id)),
			),
		)
		b.sendKb(chatID, fmt.Sprintf("«%s» — что меняем?", p.Name), &kb)

	case "editf":
		if len(parts) < 3 {
			return
		}
		b.startEditField(ctx, log, chatID, argAt(1), parts[2])

	case "hide":
		id := argAt(1)
		p, err := b.repo.GetProduct(ctx, id)
		if err != nil {
			b.adminError(log, chatID, "get product", err)
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
		b.sendKb(chatID, fmt.Sprintf("%s товар «%s»?", verb, p.Name), &kb)

	case "hideok":
		id := argAt(1)
		p, err := b.repo.GetProduct(ctx, id)
		if err != nil {
			b.adminError(log, chatID, "get product", err)
			return
		}
		if err := b.repo.UpdateProductFields(ctx, id, map[string]any{"is_hidden": !p.IsHidden}); err != nil {
			b.adminError(log, chatID, "toggle hidden", err)
			return
		}
		if p.IsHidden {
			b.send(chatID, fmt.Sprintf("👁 Товар «%s» снова виден в каталоге.", p.Name))
		} else {
			b.send(chatID, fmt.Sprintf("🙈 Товар «%s» скрыт из каталога.", p.Name))
		}

	case "del":
		id := argAt(1)
		p, err := b.repo.GetProduct(ctx, id)
		if err != nil {
			b.adminError(log, chatID, "get product", err)
			return
		}
		kb := tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🗑 Да, удалить", fmt.Sprintf("delok:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Отмена", "noop"),
		))
		b.sendKb(chatID, fmt.Sprintf("Убрать товар «%s» из каталога?\n\nОн исчезнет с витрины, но останется в истории прошлых заказов.", p.Name), &kb)

	case "delok":
		if err := b.repo.DeleteProduct(ctx, argAt(1)); err != nil {
			b.adminError(log, chatID, "delete product", err)
			return
		}
		b.sendTemp(chatID, "🗑 Товар убран из каталога.", 5*time.Second)

	default:
		// editcat_<id>:<catIdx>
		if strings.HasPrefix(action, "editcat_") {
			id64, err := strconv.ParseUint(strings.TrimPrefix(action, "editcat_"), 10, 32)
			if err != nil {
				return
			}
			idx := int(argAt(1))
			if idx >= len(model.Categories) {
				return
			}
			if err := b.repo.UpdateProductFields(ctx, uint(id64),
				map[string]any{"category": model.Categories[idx]}); err != nil {
				b.adminError(log, chatID, "set category", err)
				return
			}
			b.sendTemp(chatID, "✅ Категория изменена.", 5*time.Second)
		}
	}
}

func (b *Bot) startEditField(ctx context.Context, log *slog.Logger, chatID int64, id uint, field string) {
	switch field {
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
		kb := categoryKeyboard(fmt.Sprintf("editcat_%d", id))
		b.sendKb(chatID, "Выберите новую категорию:", &kb)
	case "hit":
		p, err := b.repo.GetProduct(ctx, id)
		if err != nil {
			b.adminError(log, chatID, "get product", err)
			return
		}
		if err := b.repo.UpdateProductFields(ctx, id, map[string]any{"is_hit": !p.IsHit}); err != nil {
			b.adminError(log, chatID, "toggle hit", err)
			return
		}
		if p.IsHit {
			b.sendTemp(chatID, "⭐ Бейдж «ХИТ» убран.", 5*time.Second)
		} else {
			b.sendTemp(chatID, "⭐ Бейдж «ХИТ» включён.", 5*time.Second)
		}
	case "disc":
		b.setWizard(chatID, &wizard{mode: "edit_discount", productID: id})
		b.send(chatID, "Введите процент скидки (например 10). 0 — убрать скидку.")
	case "stock":
		b.setWizard(chatID, &wizard{mode: "edit_stock", productID: id})
		b.send(chatID, "Введите остаток в штуках (например 3) или «-», чтобы не вести учёт.\n\n"+
			"Остаток уменьшается при каждом заказе и возвращается при отмене; "+
			"при ≤5 на витрине появляется бейдж «осталось N», при 0 букет нельзя заказать.")
	}
}

// ─── «Сегодня на базе» ─────────────────────────────────────────────────────

func (b *Bot) saveFresh(ctx context.Context, chatID int64, items string) {
	items = strings.TrimSpace(items)
	if items == "" {
		b.send(chatID, "Не получилось. Пришлите список цветов.")
		return
	}
	if utf8.RuneCountInString(items) > 300 {
		b.send(chatID, "Список — не более 300 символов.")
		return
	}
	if err := b.repo.UpsertFreshToday(ctx, b.cfg.Today(), items); err != nil {
		b.adminError(b.log, chatID, "save fresh", err)
		return
	}
	b.send(chatID, "✅ «Сегодня на базе»: "+items+"\n\nБлок уже виден на главной в магазине.")
}

// ─── Мелочи форматирования ─────────────────────────────────────────────────

func formatVariants(vs []model.ProductVariant) string {
	lines := make([]string, 0, len(vs))
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

// clipLabel — подпись кнопки Telegram ограничена 64 байтами.
func clipLabel(s string) string {
	r := []rune(s)
	if len(r) <= 30 {
		return s
	}
	return string(r[:29]) + "…"
}

func capitalize(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
