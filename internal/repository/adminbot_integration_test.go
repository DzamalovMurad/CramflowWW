package repository

import (
	"testing"
	"time"

	"github.com/dzamalovmurad/cramflowww/internal/model"
)

// Интеграционные тесты команд админ-бота (/stats, /stock, /broadcast, /export).
// Запуск — как и у остальных интеграционных тестов:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/repository/ -run Integration
//
// Проверяют то, что нельзя проверить без настоящего Postgres: агрегаты с
// FILTER, LATERAL-джойн выгрузки, UPDATE ... RETURNING в очереди рассылки
// и работу уникального индекса адресатов.

// seedStatsData наполняет БД заказами с известными суммами.
func seedStatsData(t *testing.T, r *Repository) (buyer, cartOnly model.User) {
	t.Helper()

	buyerPtr, err := r.UpsertUser(1001, "Покупатель", "+79000000001")
	if err != nil {
		t.Fatalf("создание покупателя: %v", err)
	}
	cartPtr, err := r.UpsertUser(1002, "Задумался", "+79000000002")
	if err != nil {
		t.Fatalf("создание клиента с корзиной: %v", err)
	}

	product := &model.Product{
		Name: "Розы тестовые", Category: model.CategoryStandard, IsAvailable: true,
		Variants: []model.ProductVariant{{Quantity: 9, Price: 3000}},
	}
	if err := r.CreateProduct(product); err != nil {
		t.Fatalf("создание товара: %v", err)
	}
	variantID := product.Variants[0].ID

	// Доставленный заказ на 5000 и заказ в работе на 2000 — суммы разные,
	// чтобы тест поймал путаницу между «выручкой» и «в работе».
	orders := []struct {
		status string
		total  int
		source string
	}{
		{model.StatusDelivered, 5000, model.SourceInstagram},
		{model.StatusNew, 2000, model.SourceDirect},
		{model.StatusCancelled, 9999, model.SourceDirect},
	}
	for _, o := range orders {
		order := &model.Order{
			UserID: buyerPtr.ID, TotalPrice: o.total,
			DeliveryAddress: "ул. Тестовая, 1", DeliveryDate: "2030-01-01",
			DeliveryTime: "в течение часа", Status: o.status, Source: o.source,
			Items: []model.OrderItem{{
				VariantID: variantID, Quantity: 2, Price: 1500, ProductName: product.Name,
			}},
		}
		if err := r.CreateOrder(order); err != nil {
			t.Fatalf("создание заказа: %v", err)
		}
	}

	// Клиент с активностью в корзине, но без покупок.
	if err := r.TouchCart(1002); err != nil {
		t.Fatalf("отметка корзины: %v", err)
	}
	return *buyerPtr, *cartPtr
}

func TestIntegrationStatsSummary(t *testing.T) {
	r := testRepo(t)
	resetTables(t, r)
	seedStatsData(t, r)

	from := time.Now().AddDate(0, 0, -1)
	s, err := r.GetStatsSummary(from)
	if err != nil {
		t.Fatalf("сводка: %v", err)
	}

	if s.DoneOrders != 1 || s.DoneRevenue != 5000 {
		t.Errorf("доставлено = %d заказов на %d, ожидалось 1 на 5000", s.DoneOrders, s.DoneRevenue)
	}
	if s.ActiveOrders != 1 || s.ActiveRevenue != 2000 {
		t.Errorf("в работе = %d заказов на %d, ожидалось 1 на 2000", s.ActiveOrders, s.ActiveRevenue)
	}
	if s.Cancelled != 1 {
		t.Errorf("отменено = %d, ожидалось 1", s.Cancelled)
	}
	// Отменённый заказ на 9999 не должен попадать в средний чек.
	if s.AvgCheck != 5000 {
		t.Errorf("средний чек = %.0f, ожидалось 5000", s.AvgCheck)
	}
	if s.NewUsers != 2 {
		t.Errorf("новых клиентов = %d, ожидалось 2", s.NewUsers)
	}
}

func TestIntegrationTopProductsAndSources(t *testing.T) {
	r := testRepo(t)
	resetTables(t, r)
	seedStatsData(t, r)
	from := time.Now().AddDate(0, 0, -1)

	top, err := r.GetTopProducts(from, 3)
	if err != nil {
		t.Fatalf("топ товаров: %v", err)
	}
	if len(top) != 1 {
		t.Fatalf("товаров в топе = %d, ожидался 1", len(top))
	}
	// Два живых заказа по 2 штуки; отменённый в топ не входит.
	if top[0].Qty != 4 {
		t.Errorf("продано = %d шт, ожидалось 4 (отменённый заказ не считается)", top[0].Qty)
	}

	sources, err := r.GetSourceStats(from)
	if err != nil {
		t.Fatalf("источники: %v", err)
	}
	got := map[string]int64{}
	for _, s := range sources {
		got[s.Source] = s.Orders
	}
	if got[model.SourceInstagram] != 1 || got[model.SourceDirect] != 1 {
		t.Errorf("каналы = %v, ожидалось по одному заказу у instagram и direct", got)
	}
}

func TestIntegrationExportOrders(t *testing.T) {
	r := testRepo(t)
	resetTables(t, r)
	seedStatsData(t, r)

	rows, err := r.ExportOrders(time.Time{})
	if err != nil {
		t.Fatalf("выгрузка: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("строк выгрузки = %d, ожидалось 3", len(rows))
	}
	// LATERAL-подзапрос обязан собрать состав заказа одной строкой.
	if rows[0].Items == "" {
		t.Error("состав заказа пуст — string_agg не собрал позиции")
	}
	if rows[0].ItemsTotal != 3000 { // 2 шт × 1500
		t.Errorf("сумма позиций = %d, ожидалось 3000", rows[0].ItemsTotal)
	}
	if rows[0].ClientName == "" {
		t.Error("имя клиента пусто — JOIN users не отработал")
	}
}

func TestIntegrationSegments(t *testing.T) {
	r := testRepo(t)
	resetTables(t, r)
	seedStatsData(t, r)

	all, err := r.CountSegment(model.SegmentAll)
	if err != nil {
		t.Fatalf("сегмент «все»: %v", err)
	}
	if all != 2 {
		t.Errorf("сегмент «все» = %d, ожидалось 2", all)
	}

	buyers, err := r.CountSegment(model.SegmentBuyers)
	if err != nil {
		t.Fatalf("сегмент «покупали»: %v", err)
	}
	if buyers != 1 {
		t.Errorf("сегмент «покупали» = %d, ожидался 1", buyers)
	}

	cart, err := r.CountSegment(model.SegmentCartNoOrder)
	if err != nil {
		t.Fatalf("сегмент «корзина без заказа»: %v", err)
	}
	if cart != 1 {
		t.Errorf("сегмент «корзина без заказа» = %d, ожидался 1", cart)
	}
}

// TestIntegrationBroadcastLifecycle — полный цикл рассылки: постановка в
// очередь, выдача адресатов, отметка исходов, счётчики.
func TestIntegrationBroadcastLifecycle(t *testing.T) {
	r := testRepo(t)
	resetTables(t, r)
	seedStatsData(t, r)

	bc := &model.Broadcast{AdminID: 1, Segment: model.SegmentAll, Text: "Привет", Status: model.BroadcastDraft}
	if err := r.CreateBroadcast(bc); err != nil {
		t.Fatalf("создание рассылки: %v", err)
	}

	total, err := r.QueueBroadcast(bc.ID, bc.Segment)
	if err != nil {
		t.Fatalf("постановка в очередь: %v", err)
	}
	if total != 2 {
		t.Fatalf("адресатов = %d, ожидалось 2", total)
	}

	// Повторное подтверждение не должно ни задвоить адресатов, ни пройти.
	if _, err := r.QueueBroadcast(bc.ID, bc.Segment); err == nil {
		t.Error("повторная постановка в очередь прошла — защита от двойного запуска не работает")
	}
	progress, err := r.BroadcastProgress(bc.ID)
	if err != nil {
		t.Fatalf("прогресс: %v", err)
	}
	if progress.Total != 2 {
		t.Errorf("после повторной постановки адресатов = %d, ожидалось 2", progress.Total)
	}

	if ok, err := r.ClaimBroadcast(bc.ID); err != nil || !ok {
		t.Fatalf("захват рассылки: ok=%v err=%v", ok, err)
	}
	// Второй захват обязан вернуть false: иначе два отправщика дублируют письма.
	if ok, _ := r.ClaimBroadcast(bc.ID); ok {
		t.Error("рассылку удалось захватить дважды")
	}

	batch, err := r.NextRecipients(bc.ID, 10)
	if err != nil {
		t.Fatalf("выборка адресатов: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("выдано адресатов = %d, ожидалось 2", len(batch))
	}
	// Выданные строки уже помечены sending — повторная выборка пуста.
	again, err := r.NextRecipients(bc.ID, 10)
	if err != nil {
		t.Fatalf("повторная выборка: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("повторная выборка вернула %d адресатов — возможна двойная отправка", len(again))
	}

	if err := r.MarkRecipient(batch[0].ID, model.RecipientSent, ""); err != nil {
		t.Fatalf("отметка отправки: %v", err)
	}
	if err := r.MarkRecipient(batch[1].ID, model.RecipientBlocked, "blocked"); err != nil {
		t.Fatalf("отметка блокировки: %v", err)
	}
	if err := r.MarkUserBlocked(batch[1].UserID); err != nil {
		t.Fatalf("пометка пользователя: %v", err)
	}

	progress, err = r.BroadcastProgress(bc.ID)
	if err != nil {
		t.Fatalf("прогресс: %v", err)
	}
	if progress.Sent != 1 || progress.Blocked != 1 || progress.Pending != 0 {
		t.Errorf("прогресс = %+v, ожидалось sent=1 blocked=1 pending=0", progress)
	}

	// Заблокировавший бота исключается из следующих рассылок.
	all, err := r.CountSegment(model.SegmentAll)
	if err != nil {
		t.Fatalf("сегмент после блокировки: %v", err)
	}
	if all != 1 {
		t.Errorf("сегмент «все» после блокировки = %d, ожидался 1", all)
	}
}

// TestIntegrationBroadcastResume — сценарий падения сервиса: строки, застрявшие
// в sending, закрываются и повторно НЕ отправляются, остальные дошлются.
func TestIntegrationBroadcastResume(t *testing.T) {
	r := testRepo(t)
	resetTables(t, r)
	seedStatsData(t, r)

	bc := &model.Broadcast{AdminID: 1, Segment: model.SegmentAll, Text: "Привет", Status: model.BroadcastDraft}
	if err := r.CreateBroadcast(bc); err != nil {
		t.Fatalf("создание рассылки: %v", err)
	}
	if _, err := r.QueueBroadcast(bc.ID, bc.Segment); err != nil {
		t.Fatalf("постановка в очередь: %v", err)
	}
	if _, err := r.ClaimBroadcast(bc.ID); err != nil {
		t.Fatalf("захват: %v", err)
	}

	// Один адресат «завис» в sending — процесс упал сразу после отправки.
	batch, err := r.NextRecipients(bc.ID, 1)
	if err != nil || len(batch) != 1 {
		t.Fatalf("выборка адресата: len=%d err=%v", len(batch), err)
	}

	n, err := r.RecoverInterruptedRecipients()
	if err != nil {
		t.Fatalf("восстановление: %v", err)
	}
	if n != 1 {
		t.Errorf("закрыто прерванных = %d, ожидалось 1", n)
	}

	unfinished, err := r.UnfinishedBroadcasts()
	if err != nil {
		t.Fatalf("незавершённые: %v", err)
	}
	if len(unfinished) != 1 || unfinished[0].ID != bc.ID {
		t.Fatalf("незавершённых рассылок = %d, ожидалась одна наша", len(unfinished))
	}

	// Прерванный адресат не должен попасть в повторную отправку.
	resumed, err := r.NextRecipients(bc.ID, 10)
	if err != nil {
		t.Fatalf("выборка после восстановления: %v", err)
	}
	if len(resumed) != 1 {
		t.Fatalf("к отправке осталось %d адресатов, ожидался 1", len(resumed))
	}
	if resumed[0].ID == batch[0].ID {
		t.Error("прерванный адресат выдан повторно — возможна двойная отправка")
	}
}

// TestIntegrationStockHidesFromCatalog — снятие с наличия убирает товар
// с витрины, но не ломает чтение прошлых заказов.
func TestIntegrationStockHidesFromCatalog(t *testing.T) {
	r := testRepo(t)
	resetTables(t, r)

	p := &model.Product{
		Name: "Пионы", Category: model.CategoryPremium, IsAvailable: true,
		Variants: []model.ProductVariant{{Quantity: 7, Price: 4990}},
	}
	if err := r.CreateProduct(p); err != nil {
		t.Fatalf("создание товара: %v", err)
	}
	variantID := p.Variants[0].ID

	list, err := r.ListProducts("", "", "")
	if err != nil || len(list) != 1 {
		t.Fatalf("каталог до снятия: %d товаров, err=%v", len(list), err)
	}

	if err := r.SetProductAvailable(p.ID, false); err != nil {
		t.Fatalf("снятие с наличия: %v", err)
	}

	list, err = r.ListProducts("", "", "")
	if err != nil {
		t.Fatalf("каталог после снятия: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("товар остался в каталоге после снятия с наличия (%d шт)", len(list))
	}

	gone, err := r.UnavailableVariants([]uint{variantID})
	if err != nil {
		t.Fatalf("проверка корзины: %v", err)
	}
	if len(gone) != 1 || gone[0] != variantID {
		t.Errorf("недоступные варианты = %v, ожидался [%d]", gone, variantID)
	}

	// Несуществующий вариант тоже недоступен — иначе он молча уедет в заказ.
	gone, err = r.UnavailableVariants([]uint{999999})
	if err != nil {
		t.Fatalf("проверка несуществующего варианта: %v", err)
	}
	if len(gone) != 1 {
		t.Errorf("несуществующий вариант признан доступным: %v", gone)
	}

	// Возврат в наличие возвращает товар на витрину.
	if err := r.SetProductAvailable(p.ID, true); err != nil {
		t.Fatalf("возврат в наличие: %v", err)
	}
	list, _ = r.ListProducts("", "", "")
	if len(list) != 1 {
		t.Errorf("товар не вернулся на витрину: %d шт", len(list))
	}
}

// resetTables чистит данные между тестами, сохраняя схему.
func resetTables(t *testing.T, r *Repository) {
	t.Helper()
	err := r.DB.Exec(`TRUNCATE broadcast_recipients, broadcasts, order_status_logs,
		order_items, orders, product_images, product_variants, products, users
		RESTART IDENTITY CASCADE`).Error
	if err != nil {
		t.Fatalf("очистка таблиц: %v", err)
	}
	r.InvalidateCatalog()
}
