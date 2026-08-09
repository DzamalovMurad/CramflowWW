package handler

import "sync"

// chatLocks — по мьютексу на чат.
//
// В режиме webhook Telegram присылает апдейты параллельно, и два подряд
// отправленных фото обрабатывались бы одновременно: оба обработчика писали
// бы в один и тот же черновик визарда (`w.draft.imageURLs`), теряя снимок.
// Плюс ответы бота одному админу перемешивались бы между собой.
//
// Ключи не удаляются по завершении: администраторов единицы, и рост map
// ограничен их числом.
type chatLocks struct {
	mu    sync.Mutex
	locks map[int64]*sync.Mutex
}

func newChatLocks() *chatLocks {
	return &chatLocks{locks: make(map[int64]*sync.Mutex)}
}

// Lock берёт мьютекс чата и возвращает функцию его освобождения.
func (c *chatLocks) Lock(chatID int64) func() {
	c.mu.Lock()
	m, ok := c.locks[chatID]
	if !ok {
		m = &sync.Mutex{}
		c.locks[chatID] = m
	}
	c.mu.Unlock()

	m.Lock()
	return m.Unlock
}
