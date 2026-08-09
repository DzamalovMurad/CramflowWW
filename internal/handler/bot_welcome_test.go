package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dzamalovmurad/cramflowww/internal/config"
)

// Первый экран решает, останется ли клиент. Проверяем ровно то, ради чего
// он переписан: бренд, обещание, доставка, одно действие — и ничего лишнего.
func TestWelcomeText(t *testing.T) {
	got := welcomeText("Ольга")

	for _, want := range []string{"Ольга", "Flowix", "каталог"} {
		if !strings.Contains(got, want) {
			t.Errorf("в приветствии нет %q:\n%s", want, got)
		}
	}

	// Экран читают за пару секунд — длинному тексту здесь не место,
	// и подпись к фото всё равно обрезается на 1024 символах.
	if n := len([]rune(got)); n > 400 {
		t.Errorf("приветствие разрослось до %d символов — это уже лендинг", n)
	}

	// Ни технических слов, ни перечисления возможностей.
	for _, banned := range []string{
		"Mini App", "бот", "функци", "приложени", "промокод", "статус",
		"оформ", "корзин", "профил", "API",
	} {
		if strings.Contains(strings.ToLower(got), strings.ToLower(banned)) {
			t.Errorf("в приветствии лишнее слово %q:\n%s", banned, got)
		}
	}
}

// Имя приходит из Telegram и может содержать что угодно, включая разметку.
func TestWelcomeTextEscapesName(t *testing.T) {
	got := welcomeText("<b>Оля</b> & Co")
	if strings.Contains(got, "<b>Оля") {
		t.Errorf("имя не экранировано: %s", got)
	}
	if !strings.Contains(got, "&amp;") {
		t.Errorf("амперсанд в имени не экранирован: %s", got)
	}
	// Разметка самого приветствия при этом остаётся.
	if !strings.Contains(got, "<b>Flowix</b>") {
		t.Errorf("потеряно выделение бренда: %s", got)
	}
}

// Клиент без имени в профиле не должен видеть «, это Flowix».
func TestWelcomeTextWithoutName(t *testing.T) {
	got := welcomeText("   ")
	if strings.HasPrefix(got, ",") || strings.Contains(got, " , ") {
		t.Errorf("приветствие без имени выглядит сломанным: %q", got)
	}
	if !strings.Contains(got, "Flowix") {
		t.Errorf("нет бренда: %q", got)
	}
}

// Ссылка с промокодом ведёт на тот же экран: подарок сверху, действие то же.
func TestPromoWelcomeText(t *testing.T) {
	got := promoWelcomeText("Иван", "SPRING15", "15%", " (на заказ от 3000₽)")
	for _, want := range []string{"Иван", "Flowix", "SPRING15", "15%", "3000", "каталог"} {
		if !strings.Contains(got, want) {
			t.Errorf("в приветствии с промокодом нет %q:\n%s", want, got)
		}
	}
	if n := len([]rune(got)); n > captionLimit {
		t.Errorf("подпись к фото не влезет: %d символов", n)
	}
}

// Кнопка одна, ведёт в Mini App и называется так же, как в тексте приглашения.
func TestShopKeyboard(t *testing.T) {
	b := &Bot{cfg: &config.Config{PublicURL: "https://shop.example"}}
	kb, ok := b.shopKeyboard()
	if !ok {
		t.Fatal("клавиатура не собрана при заданном PUBLIC_URL")
	}
	if len(kb.InlineKeyboard) != 1 || len(kb.InlineKeyboard[0]) != 1 {
		t.Fatalf("на первом экране должна быть ровно одна кнопка: %+v", kb.InlineKeyboard)
	}
	btn := kb.InlineKeyboard[0][0]
	if btn.Text != welcomeCTA {
		t.Errorf("подпись кнопки %q, ожидали %q", btn.Text, welcomeCTA)
	}
	if btn.WebApp.URL != "https://shop.example" {
		t.Errorf("кнопка ведёт не в Mini App: %q", btn.WebApp.URL)
	}

	// Без публичного адреса кнопку собрать не из чего — приветствие уйдёт текстом.
	empty := &Bot{cfg: &config.Config{}}
	if _, ok := empty.shopKeyboard(); ok {
		t.Error("без PUBLIC_URL клавиатуры быть не должно")
	}
}

// Текстовый запасной вариант не должен показывать клиенту теги.
func TestStripHTML(t *testing.T) {
	got := stripHTML(welcomeText("Оля"))
	if strings.Contains(got, "<") || strings.Contains(got, "&amp;") {
		t.Errorf("в текстовом варианте осталась разметка: %s", got)
	}
	if !strings.Contains(got, "Flowix") {
		t.Errorf("потерян бренд: %s", got)
	}
}

func TestWelcomeFile(t *testing.T) {
	if got := welcomeFile("https://example.com/a.jpg").SendData(); got != "https://example.com/a.jpg" {
		t.Errorf("ссылка передана как %q", got)
	}
	if got := welcomeFile("AgACAgIAAxk").SendData(); got != "AgACAgIAAxk" {
		t.Errorf("file_id передан как %q", got)
	}
	if welcomeFile("AgACAgIAAxk").NeedsUpload() {
		t.Error("file_id не должен требовать загрузки файла")
	}
	if welcomeFile("https://example.com/a.jpg").NeedsUpload() {
		t.Error("ссылку скачивает сам Telegram, загружать её не нужно")
	}
}

// Снимок первого экрана: переменная важнее файла, а несуществующий файл
// не должен превращаться в ссылку, по которой Telegram получит 404.
func TestResolveWelcomePhoto(t *testing.T) {
	dist := t.TempDir()

	// Ничего не задано и файла нет — фото не отправляем вовсе.
	cfg := &config.Config{PublicURL: "https://shop.example", WebDist: dist}
	if got := resolveWelcomePhoto(cfg); got != "" {
		t.Errorf("без файла и переменной ожидали пусто, получили %q", got)
	}

	// Файл в статике — берём его по адресу сервиса.
	if err := os.WriteFile(filepath.Join(dist, welcomeAsset), []byte("jpeg"), 0o600); err != nil {
		t.Fatalf("подготовка файла: %v", err)
	}
	if got := resolveWelcomePhoto(cfg); got != "https://shop.example/"+welcomeAsset {
		t.Errorf("ссылка на статику = %q", got)
	}

	// Лишний слэш в адресе не должен давать двойной //.
	slashed := &config.Config{PublicURL: "https://shop.example/", WebDist: dist}
	if got := resolveWelcomePhoto(slashed); got != "https://shop.example/"+welcomeAsset {
		t.Errorf("адрес со слэшем на конце = %q", got)
	}

	// Переменная перебивает файл: ею задают file_id.
	cfg.WelcomePhoto = "  AgACAgIAAxk  "
	if got := resolveWelcomePhoto(cfg); got != "AgACAgIAAxk" {
		t.Errorf("переменная не в приоритете: %q", got)
	}

	// Без публичного адреса ссылку составить не из чего.
	noURL := &config.Config{WebDist: dist}
	if got := resolveWelcomePhoto(noURL); got != "" {
		t.Errorf("без PUBLIC_URL ожидали пусто, получили %q", got)
	}
}
