package handler

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
)

// /export — выгрузка заказов в CSV. Файл открывается двойным кликом в Excel
// с русскими буквами на месте: UTF-8 BOM в начале + точка с запятой как
// разделитель (Excel в русской локали разбирает по колонкам именно её).

// Периоды выгрузки.
const (
	exportWeek  = "week"
	exportMonth = "month"
	exportAll   = "all"
)

var exportLabels = map[string]string{
	exportWeek:  "Неделя",
	exportMonth: "Месяц",
	exportAll:   "Всё",
}

var exportOrder = []string{exportWeek, exportMonth, exportAll}

func (b *Bot) sendExportMenu(chatID int64) {
	var row []tgbotapi.InlineKeyboardButton
	for _, p := range exportOrder {
		row = append(row, tgbotapi.NewInlineKeyboardButtonData(exportLabels[p], "exp:"+p))
	}
	b.sendKb(chatID, "📄 Выгрузка заказов в CSV.\nЗа какой период?",
		tgbotapi.NewInlineKeyboardMarkup(row,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✖️ Закрыть", "x"))))
}

// exportPeriodStart — начало периода выгрузки (нулевое время = все заказы).
func exportPeriodStart(period string, now time.Time) time.Time {
	switch period {
	case exportWeek:
		return now.AddDate(0, 0, -7)
	case exportMonth:
		return now.AddDate(0, 0, -30)
	default:
		return time.Time{}
	}
}

func (b *Bot) sendExport(chatID int64, period string) {
	from := exportPeriodStart(period, time.Now())

	rows, err := b.repo.ExportOrders(from)
	if err != nil {
		b.send(chatID, "Не удалось собрать выгрузку: "+err.Error())
		return
	}
	if len(rows) == 0 {
		b.send(chatID, "За этот период заказов нет.")
		return
	}

	data, err := buildOrdersCSV(rows)
	if err != nil {
		b.send(chatID, "Ошибка формирования файла: "+err.Error())
		return
	}

	name := fmt.Sprintf("flowix-orders-%s-%s.csv", period, time.Now().Format("2006-01-02"))
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FileBytes{Name: name, Bytes: data})
	doc.Caption = fmt.Sprintf("📄 %s · заказов: %s",
		exportLabels[period], model.FormatNumber(int64(len(rows))))
	m, err := b.tgSend(doc)
	if err != nil {
		b.send(chatID, "Не удалось отправить файл: "+err.Error())
		return
	}
	b.remember(chatID, m.MessageID)
}

// csvHeader — колонки выгрузки. Порядок фиксирован: на файл настраивают
// сводные таблицы, и перестановка колонок ломает их молча.
var csvHeader = []string{
	"Заказ", "Создан", "Дата доставки", "Слот", "Статус",
	"Состав", "Сумма позиций", "Итого", "Промокод", "Скидка %",
	"Источник", "Клиент", "Телефон клиента",
	"Получатель", "Телефон получателя", "Адрес",
	"Анонимно", "Причина отмены",
}

// buildOrdersCSV собирает CSV с BOM. Суммы пишем целыми числами без пробелов
// и знака валюты: иначе Excel считает колонку текстом и не даёт её просуммировать.
func buildOrdersCSV(rows []repository.ExportRow) ([]byte, error) {
	var buf bytes.Buffer
	// UTF-8 BOM: без него Excel читает файл в системной кодировке и вместо
	// кириллицы показывает «Ð Ð¾Ð·Ñ‹».
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(&buf)
	w.Comma = ';'
	if err := w.Write(csvHeader); err != nil {
		return nil, err
	}

	for _, r := range rows {
		promo := r.PromoCode
		percent := ""
		if promo != "" {
			percent = strconv.Itoa(r.PromoPercent)
		}
		anon := ""
		if r.IsAnonymous {
			anon = "да"
		}
		record := []string{
			strconv.FormatUint(uint64(r.ID), 10),
			r.CreatedAt.Format("2006-01-02 15:04"),
			r.DeliveryDate,
			r.DeliveryTime,
			model.StatusLabels[r.Status],
			r.Items,
			strconv.FormatInt(r.ItemsTotal, 10),
			strconv.FormatInt(r.TotalPrice, 10),
			promo,
			percent,
			model.SourceLabel(r.Source),
			r.ClientName,
			r.ClientPhone,
			r.RecipientName,
			r.RecipientPhone,
			r.DeliveryAddress,
			anon,
			r.CancelReason,
		}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
