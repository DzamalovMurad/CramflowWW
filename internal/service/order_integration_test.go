package service_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/dzamalovmurad/cramflowww/internal/config"
	"github.com/dzamalovmurad/cramflowww/internal/model"
	"github.com/dzamalovmurad/cramflowww/internal/repository"
	"github.com/dzamalovmurad/cramflowww/internal/service"
	"github.com/dzamalovmurad/cramflowww/internal/testdb"
)

// Интеграционные тесты денежного пути. Запуск:
//
//	TEST_DATABASE_URL=postgres://... go test ./internal/service/ -run Integration
//
// Проверяется именно то, что стоит реальных денег: сумма заказа, скидка,
// идемпотентность, лимиты промокодов и остатки при одновременных заказах.

type harness struct {
	db   *gorm.DB
	repo *repository.Repository
	svc  *service.Service
	cfg  *config.Config
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	db := testdb.Open(t)

	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		t.Skipf("нет базы часовых поясов: %v", err)
	}
	cfg := &config.Config{
		Location: loc, ShopOpenHour: 0, ShopCloseHour: 23,
		InitDataTTL: 24 * time.Hour,
	}
	repo := repository.New(db, cfg.Now)
	svc := service.New(repo, cfg, slog.New(slog.DiscardHandler))
	return &harness{db: db, repo: repo, svc: svc, cfg: cfg}
}

// product заводит товар с одним вариантом и возвращает id варианта.
// stock < 0 означает «учёт не ведётся» (в БД NULL).
func (h *harness) product(t *testing.T, name string, price, stock int) uint {
	t.Helper()
	p := &model.Product{
		Name: name, Category: model.CategoryStandard,
		Variants: []model.ProductVariant{{Quantity: 9, Price: price}},
	}
	if stock >= 0 {
		p.Stock = &stock
	}
	if err := h.repo.CreateProduct(t.Context(), p); err != nil {
		t.Fatalf("создание товара: %v", err)
	}
	return p.Variants[0].ID
}

func (h *harness) promo(t *testing.T, code string, percent, maxUses, perUser int) uint {
	t.Helper()
	p := model.PromoCode{
		Code: code, DiscountPercent: percent, MaxUses: maxUses,
		PerUserLimit: perUser, IsActive: true,
	}
	if err := h.db.Create(&p).Error; err != nil {
		t.Fatalf("создание промокода: %v", err)
	}
	return p.ID
}

// order — заказ «на завтра ко времени», чтобы тесты не зависели от часа запуска.
func order(variantID uint, qty int, tgID int64) service.OrderInput {
	return service.OrderInput{
		Items:           []service.OrderItemInput{{VariantID: variantID, Quantity: qty}},
		Name:            "Иван Тестов",
		Phone:           "+79000000000",
		DeliveryAddress: "Москва, ул. Тверская, 1",
		DeliveryDate:    time.Now().AddDate(0, 0, 1).Format("2006-01-02"),
		DeliveryTime:    "к 15:00",
		TelegramID:      tgID,
	}
}

// ─── Деньги ────────────────────────────────────────────────────────────────

// Сумма считается по ценам из БД, а не по тому, что прислал клиент.
func TestIntegrationOrderTotalFromDatabase(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Розы", 2990, -1)

	o, err := h.svc.CreateOrder(t.Context(), order(v, 3, 1001))
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}
	if o.SubtotalPrice != 8970 || o.TotalPrice != 8970 || o.DiscountAmount != 0 {
		t.Fatalf("суммы: subtotal=%d discount=%d total=%d, ожидали 8970/0/8970",
			o.SubtotalPrice, o.DiscountAmount, o.TotalPrice)
	}
	if len(o.Items) != 1 || o.Items[0].Price != 2990 {
		t.Fatalf("позиция заказа записана неверно: %+v", o.Items)
	}
	// Название зафиксировано на момент заказа.
	if o.Items[0].ProductName != "Розы" {
		t.Errorf("название товара не зафиксировано: %q", o.Items[0].ProductName)
	}
}

// Изменение цены после добавления в корзину не должно ломать инвариант
// «сумма заказа = сумма позиций»: считаем по актуальной цене.
func TestIntegrationOrderUsesCurrentPrice(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Тюльпаны", 2000, -1)

	if err := h.db.Model(&model.ProductVariant{}).Where("id = ?", v).
		Update("price", 2500).Error; err != nil {
		t.Fatalf("правка цены: %v", err)
	}

	o, err := h.svc.CreateOrder(t.Context(), order(v, 2, 1002))
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}
	if o.TotalPrice != 5000 {
		t.Fatalf("итог = %d, ожидали 5000 по новой цене", o.TotalPrice)
	}
}

func TestIntegrationOrderWithPromo(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Пионы", 4990, -1)
	h.promo(t, "SPRING15", 15, 0, 0)

	in := order(v, 1, 1003)
	in.PromoCode = "spring15" // регистр не важен
	o, err := h.svc.CreateOrder(t.Context(), in)
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}
	// 4990 * 15 / 100 = 748 (округление вниз)
	if o.SubtotalPrice != 4990 || o.DiscountAmount != 748 || o.TotalPrice != 4242 {
		t.Fatalf("суммы со скидкой: %d/%d/%d, ожидали 4990/748/4242",
			o.SubtotalPrice, o.DiscountAmount, o.TotalPrice)
	}
	if o.SubtotalPrice-o.DiscountAmount != o.TotalPrice {
		t.Error("нарушен инвариант total = subtotal − discount")
	}
	if o.PromoCode == nil || o.PromoCode.Code != "SPRING15" {
		t.Error("промокод не записан в заказ")
	}
}

// ─── Идемпотентность ───────────────────────────────────────────────────────

// Повторная отправка формы с тем же ключом не должна создавать второй заказ.
func TestIntegrationOrderIdempotency(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Ромашки", 1500, -1)

	in := order(v, 1, 1004)
	in.IdempotencyKey = "form-attempt-1"

	first, err := h.svc.CreateOrder(t.Context(), in)
	if err != nil {
		t.Fatalf("первый заказ: %v", err)
	}
	second, err := h.svc.CreateOrder(t.Context(), in)
	if err != nil {
		t.Fatalf("повтор: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("повтор создал новый заказ: #%d и #%d", first.ID, second.ID)
	}

	var count int64
	h.db.Model(&model.Order{}).Count(&count)
	if count != 1 {
		t.Fatalf("в БД %d заказов, ожидали 1", count)
	}
}

// Двойной тап: два одновременных запроса с одним ключом.
func TestIntegrationOrderIdempotencyConcurrent(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Гвоздики", 1200, -1)

	in := order(v, 1, 1005)
	in.IdempotencyKey = "double-tap"

	const n = 5
	ids := make([]uint, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			o, err := h.svc.CreateOrder(context.Background(), in)
			if err != nil {
				errs[i] = err
				return
			}
			ids[i] = o.ID
		}(i)
	}
	close(start)
	wg.Wait()

	var count int64
	h.db.Model(&model.Order{}).Count(&count)
	if count != 1 {
		t.Fatalf("создано %d заказов при одном ключе идемпотентности, ошибки: %v", count, errs)
	}
	for i, id := range ids {
		if errs[i] == nil && id != ids[0] && ids[0] != 0 {
			t.Errorf("запрос %d вернул другой заказ #%d вместо #%d", i, id, ids[0])
		}
	}
}

// ─── Промокоды под нагрузкой ───────────────────────────────────────────────

// max_uses не должен пробиваться одновременными заказами.
func TestIntegrationPromoMaxUsesUnderRace(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Хризантемы", 1000, -1)
	h.promo(t, "LIMIT3", 20, 3, 0) // всего 3 применения, без личного лимита

	const n = 10
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := order(v, 1, int64(2000+i)) // разные клиенты
			in.PromoCode = "LIMIT3"
			<-start
			_, _ = h.svc.CreateOrder(context.Background(), in)
		}(i)
	}
	close(start)
	wg.Wait()

	var uses int
	h.db.Model(&model.PromoCode{}).Where("code = ?", "LIMIT3").Select("uses").Scan(&uses)
	if uses > 3 {
		t.Fatalf("промокод применён %d раз при лимите 3", uses)
	}

	var redemptions int64
	h.db.Model(&model.PromoRedemption{}).Count(&redemptions)
	if int(redemptions) != uses {
		t.Fatalf("uses=%d, но записей о списании %d — счётчик разошёлся", uses, redemptions)
	}

	// Заказы со скидкой ровно те, что попали в лимит.
	var discounted int64
	h.db.Model(&model.Order{}).Where("promo_code_id IS NOT NULL").Count(&discounted)
	if int(discounted) != uses {
		t.Fatalf("заказов со скидкой %d, применений промокода %d", discounted, uses)
	}
}

// Личный лимит: один клиент не применит код дважды.
func TestIntegrationPromoPerUserLimit(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Лилии", 3000, -1)
	h.promo(t, "ONCE", 10, 0, 1)

	in := order(v, 1, 3001)
	in.PromoCode = "ONCE"
	if _, err := h.svc.CreateOrder(t.Context(), in); err != nil {
		t.Fatalf("первый заказ: %v", err)
	}

	in2 := order(v, 1, 3001)
	in2.PromoCode = "ONCE"
	if _, err := h.svc.CreateOrder(t.Context(), in2); err == nil {
		t.Fatal("повторное применение личного промокода должно отклоняться")
	}

	// А другому клиенту — можно.
	in3 := order(v, 1, 3002)
	in3.PromoCode = "ONCE"
	if _, err := h.svc.CreateOrder(t.Context(), in3); err != nil {
		t.Fatalf("другому клиенту код должен подойти: %v", err)
	}
}

func TestIntegrationPromoInactiveAndExpired(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Астры", 900, -1)

	past := time.Now().Add(-time.Hour)
	h.db.Create(&model.PromoCode{Code: "OFF", DiscountPercent: 10, IsActive: false})
	h.db.Create(&model.PromoCode{Code: "OLD", DiscountPercent: 10, IsActive: true, ExpiresAt: &past})

	for _, code := range []string{"OFF", "OLD", "NOSUCH"} {
		in := order(v, 1, 3100)
		in.PromoCode = code
		if _, err := h.svc.CreateOrder(t.Context(), in); err == nil {
			t.Errorf("промокод %s не должен применяться", code)
		}
	}
}

// ─── Остатки ───────────────────────────────────────────────────────────────

// Ограниченный остаток не должен уходить в минус при одновременных заказах.
func TestIntegrationStockUnderRace(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Редкая орхидея", 9900, 5)

	const n = 12
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, _ = h.svc.CreateOrder(context.Background(), order(v, 1, int64(4000+i)))
		}(i)
	}
	close(start)
	wg.Wait()

	var stock int
	h.db.Model(&model.Product{}).Where("name = ?", "Редкая орхидея").Select("stock").Scan(&stock)
	if stock < 0 {
		t.Fatalf("остаток ушёл в минус: %d", stock)
	}
	_ = stock

	var orders int64
	h.db.Model(&model.Order{}).Count(&orders)
	if orders > 5 {
		t.Fatalf("создано %d заказов при остатке 5", orders)
	}
	if int(orders)+stock != 5 {
		t.Fatalf("заказов %d + остаток %d ≠ 5", orders, stock)
	}
}

// Товар без учёта остатка (NULL) заказывается сколько угодно раз.
func TestIntegrationUnlimitedStock(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Обычные розы", 2000, -1)

	for i := 0; i < 3; i++ {
		if _, err := h.svc.CreateOrder(t.Context(), order(v, 10, int64(4100+i))); err != nil {
			t.Fatalf("заказ %d: %v", i, err)
		}
	}
	var stock *int
	h.db.Model(&model.Product{}).Where("name = ?", "Обычные розы").Select("stock").Scan(&stock)
	if stock != nil {
		t.Fatalf("остаток без учёта стал числом: %d", *stock)
	}
}

// Заказ, купивший последний товар, обнуляет остаток; отмена возвращает его.
func TestIntegrationStockRestoredOnCancel(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Единственный букет", 5000, 2)

	o, err := h.svc.CreateOrder(t.Context(), order(v, 2, 4300))
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}
	if stock := h.stockOf(t, "Единственный букет"); stock != 0 {
		t.Fatalf("после заказа остаток = %d, ожидали 0", stock)
	}
	// Закончившийся товар больше не заказать.
	if _, err := h.svc.CreateOrder(t.Context(), order(v, 1, 4301)); err == nil {
		t.Fatal("товар с нулевым остатком не должен заказываться")
	}

	if _, err := h.svc.TransitionOrder(t.Context(), o.ID, model.StatusCancelled, 1, "нет цветов"); err != nil {
		t.Fatalf("отмена: %v", err)
	}
	if stock := h.stockOf(t, "Единственный букет"); stock != 2 {
		t.Fatalf("после отмены остаток = %d, ожидали 2", stock)
	}
	// И снова доступен.
	if _, err := h.svc.CreateOrder(t.Context(), order(v, 1, 4302)); err != nil {
		t.Fatalf("после возврата остатка заказ должен проходить: %v", err)
	}
}

func (h *harness) stockOf(t *testing.T, name string) int {
	t.Helper()
	var stock *int
	if err := h.db.Model(&model.Product{}).Where("name = ?", name).
		Select("stock").Scan(&stock).Error; err != nil {
		t.Fatalf("чтение остатка: %v", err)
	}
	if stock == nil {
		t.Fatalf("у товара %q остаток не ведётся", name)
	}
	return *stock
}

// Заказ больше, чем есть на складе, отклоняется целиком.
func TestIntegrationStockInsufficient(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Последние тюльпаны", 1000, 2)

	if _, err := h.svc.CreateOrder(t.Context(), order(v, 5, 4200)); err == nil {
		t.Fatal("заказ на 5 при остатке 2 должен отклоняться")
	}
	var orders int64
	h.db.Model(&model.Order{}).Count(&orders)
	if orders != 0 {
		t.Fatalf("отклонённый заказ всё равно создался (%d шт)", orders)
	}
}

// ─── Недоступные товары ────────────────────────────────────────────────────

func TestIntegrationHiddenAndDeletedProducts(t *testing.T) {
	h := newHarness(t)
	hidden := h.product(t, "Скрытый", 1000, -1)
	deleted := h.product(t, "Удалённый", 1000, -1)

	var hiddenProductID, deletedProductID uint
	h.db.Model(&model.ProductVariant{}).Where("id = ?", hidden).Select("product_id").Scan(&hiddenProductID)
	h.db.Model(&model.ProductVariant{}).Where("id = ?", deleted).Select("product_id").Scan(&deletedProductID)

	if err := h.repo.UpdateProductFields(t.Context(), hiddenProductID,
		map[string]any{"is_hidden": true}); err != nil {
		t.Fatalf("скрытие: %v", err)
	}
	if err := h.repo.DeleteProduct(t.Context(), deletedProductID); err != nil {
		t.Fatalf("удаление: %v", err)
	}

	for name, variantID := range map[string]uint{"скрытый": hidden, "удалённый": deleted} {
		_, err := h.svc.CreateOrder(t.Context(), order(variantID, 1, 5000))
		if err == nil {
			t.Errorf("%s товар не должен заказываться", name)
			continue
		}
		var ve *service.ValidationError
		if !errors.As(err, &ve) || len(ve.UnavailableVariants) != 1 {
			t.Errorf("%s товар: ожидали список недоступных позиций, получили %v", name, err)
		}
	}
}

// ─── Конечный автомат в БД ─────────────────────────────────────────────────

func TestIntegrationOrderStateMachine(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Букет для теста статусов", 1000, -1)
	o, err := h.svc.CreateOrder(t.Context(), order(v, 1, 6000))
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}

	// Прыжок через шаг запрещён.
	if _, err := h.svc.TransitionOrder(t.Context(), o.ID, model.StatusDelivered, 1, ""); err == nil {
		t.Error("прыжок new → delivered должен отклоняться")
	}

	status := model.StatusNew
	for {
		next := model.NextStatus(status)
		if next == "" {
			break
		}
		updated, err := h.svc.TransitionOrder(t.Context(), o.ID, next, 42, "")
		if err != nil {
			t.Fatalf("переход %s → %s: %v", status, next, err)
		}
		if updated.Status != next {
			t.Fatalf("статус не сохранился: %q вместо %q", updated.Status, next)
		}
		status = next
	}
	if status != model.StatusDelivered {
		t.Fatalf("конвейер остановился на %q", status)
	}

	// Доставленный заказ отменить нельзя.
	if _, err := h.svc.TransitionOrder(t.Context(), o.ID, model.StatusCancelled, 42, "передумали"); err == nil {
		t.Error("отмена доставленного заказа должна отклоняться")
	}

	// Журнал заполнен на каждый переход.
	var logs int64
	h.db.Model(&model.OrderStatusLog{}).Where("order_id = ?", o.ID).Count(&logs)
	if logs != 4 {
		t.Errorf("в журнале %d записей, ожидали 4", logs)
	}
}

func TestIntegrationCancelRequiresReason(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Букет для отмены", 1000, -1)
	o, err := h.svc.CreateOrder(t.Context(), order(v, 1, 6100))
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}
	if _, err := h.svc.TransitionOrder(t.Context(), o.ID, model.StatusCancelled, 1, "   "); err == nil {
		t.Error("отмена без причины должна отклоняться")
	}
	updated, err := h.svc.TransitionOrder(t.Context(), o.ID, model.StatusCancelled, 1, "клиент передумал")
	if err != nil {
		t.Fatalf("отмена с причиной: %v", err)
	}
	if updated.CancelReason != "клиент передумал" {
		t.Errorf("причина отмены не сохранена: %q", updated.CancelReason)
	}
}

// Два админа жмут «дальше» одновременно — статус должен сдвинуться один раз.
func TestIntegrationConcurrentStatusChange(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Букет для гонки статусов", 1000, -1)
	o, err := h.svc.CreateOrder(t.Context(), order(v, 1, 6200))
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}

	const n = 6
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if _, err := h.svc.TransitionOrder(context.Background(), o.ID,
				model.StatusConfirmed, int64(i), ""); err == nil {
				mu.Lock()
				ok++
				mu.Unlock()
			}
		}(i)
	}
	close(start)
	wg.Wait()

	if ok != 1 {
		t.Fatalf("успешных переходов %d, ожидали ровно 1", ok)
	}
	var logs int64
	h.db.Model(&model.OrderStatusLog{}).Where("order_id = ?", o.ID).Count(&logs)
	if logs != 1 {
		t.Fatalf("в журнале %d записей, ожидали 1", logs)
	}
}

// ─── Уведомление админам ───────────────────────────────────────────────────

// Уведомление уходит после успешного заказа и не мешает ответу клиенту.
func TestIntegrationNotifyScheduledAfterOrder(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Букет для уведомления", 1000, -1)

	notified := make(chan uint, 1)
	h.svc.NotifyNewOrder = func(o *model.Order) { notified <- o.ID }

	o, err := h.svc.CreateOrder(t.Context(), order(v, 1, 7000))
	if err != nil {
		t.Fatalf("создание заказа: %v", err)
	}
	select {
	case id := <-notified:
		if id != o.ID {
			t.Errorf("уведомили о заказе #%d вместо #%d", id, o.ID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("уведомление о заказе не отправлено")
	}
}

// Паника в уведомлении не должна ронять процесс и не должна влиять на заказ.
func TestIntegrationNotifyPanicIsolated(t *testing.T) {
	h := newHarness(t)
	v := h.product(t, "Букет для паники", 1000, -1)

	done := make(chan struct{})
	h.svc.NotifyNewOrder = func(*model.Order) {
		defer close(done)
		panic("уведомлятор сломался")
	}

	o, err := h.svc.CreateOrder(t.Context(), order(v, 1, 7100))
	if err != nil {
		t.Fatalf("заказ не должен зависеть от уведомления: %v", err)
	}
	if o.ID == 0 {
		t.Fatal("заказ не создан")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("уведомление не вызвано")
	}
	time.Sleep(100 * time.Millisecond) // дать recover сработать
}

// Неудачный заказ не должен оставлять следов: ни позиций, ни списания промокода.
func TestIntegrationFailedOrderLeavesNothing(t *testing.T) {
	h := newHarness(t)
	good := h.product(t, "Есть", 1000, -1)
	h.promo(t, "ROLLBACK", 10, 0, 0)

	in := order(good, 1, 8000)
	in.Items = append(in.Items, service.OrderItemInput{VariantID: 999999, Quantity: 1})
	in.PromoCode = "ROLLBACK"

	if _, err := h.svc.CreateOrder(t.Context(), in); err == nil {
		t.Fatal("заказ с несуществующей позицией должен отклоняться")
	}

	for table, dest := range map[string]any{
		"orders":            &model.Order{},
		"order_items":       &model.OrderItem{},
		"promo_redemptions": &model.PromoRedemption{},
	} {
		var n int64
		h.db.Model(dest).Count(&n)
		if n != 0 {
			t.Errorf("после отката в %s осталось %d записей", table, n)
		}
	}
	var uses int
	h.db.Model(&model.PromoCode{}).Where("code = ?", "ROLLBACK").Select("uses").Scan(&uses)
	if uses != 0 {
		t.Errorf("счётчик промокода вырос до %d при откате", uses)
	}
}

// Заказы с одними и теми же товарами в разном порядке не должны
// блокировать друг друга крест-накрест: без сортировки id при списании
// остатков Postgres разрывает такую пару откатом («deadlock detected»),
// и клиент получает ошибку на ровном месте.
func TestIntegrationConcurrentOrdersNoDeadlock(t *testing.T) {
	h := newHarness(t)
	a := h.product(t, "Товар А", 1000, 500)
	b := h.product(t, "Товар Б", 2000, 500)
	c := h.product(t, "Товар В", 3000, 500)

	// Каждая горутина складывает те же три товара в своём порядке.
	orders := [][]uint{{a, b, c}, {c, b, a}, {b, a, c}, {c, a, b}, {a, c, b}, {b, c, a}}

	const rounds = 6
	var wg sync.WaitGroup
	errs := make(chan error, len(orders)*rounds)
	start := make(chan struct{})

	for round := 0; round < rounds; round++ {
		for i, variantOrder := range orders {
			wg.Add(1)
			go func(round, i int, variantOrder []uint) {
				defer wg.Done()
				in := order(variantOrder[0], 1, int64(90000+round*100+i))
				in.Items = nil
				for _, v := range variantOrder {
					in.Items = append(in.Items, service.OrderItemInput{VariantID: v, Quantity: 1})
				}
				<-start
				if _, err := h.svc.CreateOrder(context.Background(), in); err != nil {
					errs <- err
				}
			}(round, i, variantOrder)
		}
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("заказ не прошёл: %v", err)
	}

	var count int64
	h.db.Model(&model.Order{}).Count(&count)
	if count != int64(len(orders)*rounds) {
		t.Fatalf("создано %d заказов из %d", count, len(orders)*rounds)
	}
	// Остатки списаны ровно по разу на заказ.
	for _, name := range []string{"Товар А", "Товар Б", "Товар В"} {
		if got := h.stockOf(t, name); got != 500-len(orders)*rounds {
			t.Errorf("%s: остаток %d, ожидали %d", name, got, 500-len(orders)*rounds)
		}
	}
}
