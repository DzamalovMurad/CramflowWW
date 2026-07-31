package model

import "testing"

func TestFormatMoney(t *testing.T) {
	cases := map[int64]string{
		0:       "0 ₽",
		5:       "5 ₽",
		999:     "999 ₽",
		1000:    "1 000 ₽",
		12340:   "12 340 ₽",
		100000:  "100 000 ₽",
		1234567: "1 234 567 ₽",
		-2500:   "−2 500 ₽",
	}
	for in, want := range cases {
		if got := FormatMoney(in); got != want {
			t.Errorf("FormatMoney(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeSource(t *testing.T) {
	cases := map[string]string{
		"":                   SourceDirect,
		"   ":                SourceDirect,
		"Instagram":          "instagram",
		"INSTAGRAM":          "instagram",
		"vk_stories":         "vk_stories",
		"promo-2026":         "promo-2026",
		"инстаграм":          SourceDirect, // кириллица отсекается целиком
		"drop table orders;": "droptableorders",
		"--':":               SourceDirect,
		"a_very_long_source_label_that_exceeds_the_limit": "a_very_long_source_label_that_ex", // 32 символа
	}
	for in, want := range cases {
		if got := NormalizeSource(in); got != want {
			t.Errorf("NormalizeSource(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSourceLabel(t *testing.T) {
	if got := SourceLabel(SourceInstagram); got != "Instagram" {
		t.Errorf("SourceLabel(instagram) = %q", got)
	}
	// Незнакомая метка показывается как есть — метки заводит маркетолог, не код.
	if got := SourceLabel("promo-2026"); got != "promo-2026" {
		t.Errorf("SourceLabel(promo-2026) = %q", got)
	}
	if got := SourceLabel(""); got != "Напрямую" {
		t.Errorf("SourceLabel(\"\") = %q", got)
	}
}

func TestBroadcastProgressDone(t *testing.T) {
	p := BroadcastProgress{Total: 10, Pending: 2, Sent: 5, Failed: 2, Blocked: 1}
	if p.Done() != 8 {
		t.Errorf("Done() = %d, want 8", p.Done())
	}
}
