package service

import (
	"fmt"
	"time"
)

// Слоты и cutoff считаются в московском времени (UTC+3, перехода на летнее нет).
var MskLocation = time.FixedZone("MSK", 3*60*60)

// DeliverySlots — двухчасовые окна доставки; значение хранится в Order.DeliveryTime как есть.
var DeliverySlots = []string{
	"10:00-12:00", "12:00-14:00", "14:00-16:00",
	"16:00-18:00", "18:00-20:00", "20:00-22:00",
}

const (
	slotCutoffHour      = 19            // после 19:00 MSK заказы на сегодня не принимаются
	slotMinLead         = 2 * time.Hour // слот должен начинаться не раньше, чем через 2 часа
	slotMaxDays         = 7             // максимум дней вперёд
	DefaultSlotCapacity = 4             // заказов в слот (env SLOT_CAPACITY)
)

// SlotInfo — слот в выдаче /api/delivery-slots.
type SlotInfo struct {
	Slot      string `json:"slot"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"` // «уже недоступен» | «занят»
}

// SlotDay — день с набором слотов для чипов в checkout.
type SlotDay struct {
	Date  string     `json:"date"`  // YYYY-MM-DD
	Label string     `json:"label"` // «сегодня» | «завтра» | «сб, 2 авг»
	Slots []SlotInfo `json:"slots"`
}

// Подписи дат — строчными, в стиле бренда (тексты Mini App все lowercase).
var weekdayShort = [...]string{"вс", "пн", "вт", "ср", "чт", "пт", "сб"}
var monthShort = [...]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}

func dayLabel(day time.Time, offset int) string {
	switch offset {
	case 0:
		return "сегодня"
	case 1:
		return "завтра"
	}
	return fmt.Sprintf("%s, %d %s", weekdayShort[day.Weekday()], day.Day(), monthShort[day.Month()-1])
}

// slotStart — момент начала слота в MSK.
func slotStart(date, slot string) (time.Time, error) {
	if len(slot) < 5 {
		return time.Time{}, fmt.Errorf("некорректный слот %q", slot)
	}
	return time.ParseInLocation("2006-01-02 15:04", date+" "+slot[:5], MskLocation)
}

func isDeliverySlot(slot string) bool {
	for _, s := range DeliverySlots {
		if s == slot {
			return true
		}
	}
	return false
}

// DeliverySlotDays — календарь для checkout: до 7 дней вперёд с учётом cutoff
// (после 19:00 MSK сегодняшний день закрыт), правила «слот начинается не раньше
// чем через 2 часа» и занятости слотов (лимит SlotCapacity).
func (s *Service) DeliverySlotDays() ([]SlotDay, error) {
	now := time.Now().In(MskLocation)
	from := now.Format("2006-01-02")
	to := now.AddDate(0, 0, slotMaxDays).Format("2006-01-02")
	counts, err := s.Repo.CountOrdersInSlots(from, to)
	if err != nil {
		return nil, err
	}

	days := make([]SlotDay, 0, slotMaxDays+1)
	for d := 0; d <= slotMaxDays; d++ {
		if d == 0 && now.Hour() >= slotCutoffHour {
			continue // cutoff: сегодня уже не заказать
		}
		day := now.AddDate(0, 0, d)
		date := day.Format("2006-01-02")
		sd := SlotDay{Date: date, Label: dayLabel(day, d)}
		for _, slot := range DeliverySlots {
			info := SlotInfo{Slot: slot, Available: true}
			if start, err := slotStart(date, slot); err == nil && start.Before(now.Add(slotMinLead)) {
				info.Available = false
				info.Reason = "уже недоступен"
			} else if s.SlotCapacity > 0 && counts[date][slot] >= s.SlotCapacity {
				info.Available = false
				info.Reason = "занят"
			}
			sd.Slots = append(sd.Slots, info)
		}
		days = append(days, sd)
	}
	return days, nil
}

// validateSlot — серверная проверка даты и слота при создании заказа
// (клиентским чипам не доверяем). Занятость слота проверяется отдельно,
// в транзакции создания заказа.
func (s *Service) validateSlot(date, slot string) error {
	if !isDeliverySlot(slot) {
		return invalid("выберите слот доставки")
	}
	day, err := time.ParseInLocation("2006-01-02", date, MskLocation)
	if err != nil {
		return invalid("некорректная дата доставки")
	}
	now := time.Now().In(MskLocation)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, MskLocation)
	if day.Before(today) {
		return invalid("дата доставки уже прошла")
	}
	if day.After(today.AddDate(0, 0, slotMaxDays)) {
		return invalid("заказ можно оформить максимум на %d дней вперёд", slotMaxDays)
	}
	if day.Equal(today) {
		if now.Hour() >= slotCutoffHour {
			return invalid("заказы на сегодня принимаются до 19:00 — выберите другую дату")
		}
		start, err := slotStart(date, slot)
		if err != nil || start.Before(now.Add(slotMinLead)) {
			return invalid("этот слот уже недоступен — выберите более позднее время")
		}
	}
	return nil
}
