package model

import "strconv"

// FormatMoney — рубли в человеческом виде: 12340 → «12 340 ₽».
// Разделитель разрядов — обычный пробел: суммы уходят и в текст Telegram,
// и в CSV-выгрузку, а невидимые спецпробелы Excel показывает как мусор.
func FormatMoney(v int64) string {
	return FormatNumber(v) + " ₽"
}

// groupSep — разделитель разрядов. Вынесен в константу намеренно: обычный
// пробел и его «типографские» родственники (U+00A0, U+202F) на глаз неразличимы,
// и подменённый символ ломает и тесты, и колонки CSV молча.
const groupSep = " "

// FormatNumber — то же разбиение по разрядам, но без знака валюты
// (количество заказов, адресатов рассылки и т.п.).
func FormatNumber(v int64) string {
	sign := ""
	if v < 0 {
		sign = "−"
		v = -v
	}
	digits := strconv.FormatInt(v, 10)

	// Группы по три цифры справа налево.
	head := len(digits) % 3
	if head == 0 {
		head = 3
	}
	out := digits[:head]
	for i := head; i < len(digits); i += 3 {
		out += groupSep + digits[i:i+3]
	}
	return sign + out
}
