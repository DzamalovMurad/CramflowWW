package model

import (
	"strings"
	"testing"
)

// Регрессия: шаблон без %d давал в сообщении мусор «%!(EXTRA uint=4)».
func TestClientStatusTextNoFormatLeftovers(t *testing.T) {
	for status := range ClientStatusMessages {
		text, ok := ClientStatusText(status, 4)
		if !ok {
			t.Errorf("статус %s: ожидали текст", status)
			continue
		}
		if strings.Contains(text, "%!") || strings.Contains(text, "EXTRA") {
			t.Errorf("статус %s: мусор форматирования в тексте: %q", status, text)
		}
		if strings.Contains(text, "%d") {
			t.Errorf("статус %s: неподставленный %%d: %q", status, text)
		}
		if !strings.Contains(text, "4") {
			t.Errorf("статус %s: нет номера заказа: %q", status, text)
		}
	}
}

func TestClientStatusTextUnknownStatus(t *testing.T) {
	if _, ok := ClientStatusText(StatusNew, 1); ok {
		t.Error("для статуса new клиенту писать не нужно")
	}
	if _, ok := ClientStatusText("nonsense", 1); ok {
		t.Error("неизвестный статус не должен давать текст")
	}
}
