package handler

import (
	"sync"
	"testing"
)

// Апдейты одного чата обязаны обрабатываться строго по очереди: иначе два
// подряд присланных фото пишут в один черновик визарда и снимок теряется.
// Тест ловит это под -race.
func TestChatLocksSerializeSameChat(t *testing.T) {
	locks := newChatLocks()

	// Общая структура играет роль w.draft.imageURLs.
	var draft []int
	inside := 0
	maxInside := 0
	var guard sync.Mutex

	const n = 50
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			unlock := locks.Lock(42)
			defer unlock()

			guard.Lock()
			inside++
			if inside > maxInside {
				maxInside = inside
			}
			guard.Unlock()

			draft = append(draft, i) // без сериализации здесь гонка

			guard.Lock()
			inside--
			guard.Unlock()
		}(i)
	}
	close(start)
	wg.Wait()

	if maxInside != 1 {
		t.Fatalf("одновременно в критической секции было %d обработчиков, ожидали 1", maxInside)
	}
	if len(draft) != n {
		t.Fatalf("в черновике %d элементов, ожидали %d — часть записей потерялась", len(draft), n)
	}
}

// Разные чаты не должны ждать друг друга: один админ, залипший на загрузке
// фото, не может блокировать второго.
func TestChatLocksDoNotBlockOtherChats(t *testing.T) {
	locks := newChatLocks()

	held := locks.Lock(1)
	done := make(chan struct{})
	go func() {
		unlock := locks.Lock(2) // другой чат — не должен ждать
		unlock()
		close(done)
	}()

	<-done // если бы блокировал, тест завис бы и упал по таймауту
	held()
}

// Повторный захват того же чата после освобождения работает.
func TestChatLocksReusable(t *testing.T) {
	locks := newChatLocks()
	for i := 0; i < 3; i++ {
		unlock := locks.Lock(7)
		unlock()
	}
}
