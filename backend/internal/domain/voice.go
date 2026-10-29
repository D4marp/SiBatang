package domain

import (
	"strings"
	"unicode"
)

// NamePolicy per audio zone. Default aman = NumberOnly (OPEN-03).
type NamePolicy string

const (
	NameNumberOnly NamePolicy = "NUMBER_ONLY"
	NamePreferred  NamePolicy = "PREFERRED_NAME_IF_CONSENT"
	NameFull       NamePolicy = "FULL_NAME_IF_APPROVED"
)

const maxCallNameRunes = 24

// ResolveCallName menentukan nama yang boleh diucapkan. Nil = hanya nomor + loket.
// Dipanggil HANYA pada klaim announcer privat; hasilnya tidak boleh masuk SSE/outbox/log/metrik.
func ResolveCallName(policy NamePolicy, consent bool, storedName string) *string {
	if policy != NamePreferred && policy != NameFull {
		return nil
	}
	if !consent {
		return nil
	}
	name := sanitizeName(storedName)
	if name == "" {
		return nil
	}
	if policy == NamePreferred {
		name = strings.Fields(name)[0] // nama depan/panggilan saja
	}
	r := []rune(name)
	if len(r) > maxCallNameRunes {
		name = strings.TrimSpace(string(r[:maxCallNameRunes]))
	}
	return &name
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r), r == '\'', r == '-', r == '.':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

var digitWords = [...]string{"nol", "satu", "dua", "tiga", "empat", "lima", "enam", "tujuh", "delapan", "sembilan"}

// SpellDigits: "025" -> "nol dua lima" (digit per digit untuk mengurangi salah dengar).
func SpellDigits(s string) string {
	var out []string
	for _, r := range s {
		if r >= '0' && r <= '9' {
			out = append(out, digitWords[r-'0'])
		}
	}
	return strings.Join(out, " ")
}

// SpellLetters: "SKCK" -> "S K C K" (pengejaan prefiks layanan, bukan TTS bebas).
func SpellLetters(s string) string {
	var out []string
	for _, r := range s {
		if unicode.IsLetter(r) {
			out = append(out, strings.ToUpper(string(r)))
		}
	}
	return strings.Join(out, " ")
}

// SpokenSpeech adalah isi privat klaim announcer.
type SpokenSpeech struct {
	ServiceSpoken  string  `json:"service_spoken"`
	NumberSpoken   string  `json:"number_spoken"`
	PublicCallName *string `json:"public_call_name"`
	CounterSpoken  string  `json:"counter_spoken"`
	TemplateID     string  `json:"template_id"`
}

// BuildSpeech menyusun komponen ucapan dari nomor tampil ("SKCK-025") dan label loket ("Loket 2").
func BuildSpeech(displayNumber, counterLabel string, name *string) SpokenSpeech {
	prefix, serial := displayNumber, ""
	if i := strings.LastIndex(displayNumber, "-"); i >= 0 {
		prefix, serial = displayNumber[:i], displayNumber[i+1:]
	}
	return SpokenSpeech{
		ServiceSpoken:  SpellLetters(prefix),
		NumberSpoken:   SpellDigits(serial),
		PublicCallName: name,
		CounterSpoken:  spellCounter(counterLabel),
		TemplateID:     "id-ID-v1",
	}
}

func spellCounter(label string) string {
	var words, digits []string
	for _, f := range strings.Fields(label) {
		if isDigits(f) {
			digits = append(digits, SpellDigits(f))
		} else {
			words = append(words, strings.ToLower(f))
		}
	}
	return strings.TrimSpace(strings.Join(append(words, digits...), " "))
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Utterance = template id-ID-v1 (referensi kontrak; klien Dart harus identik, diuji di sisi klien).
func Utterance(sp SpokenSpeech) string {
	var b strings.Builder
	b.WriteString("Nomor antrean " + sp.ServiceSpoken + ", " + sp.NumberSpoken)
	if sp.PublicCallName != nil {
		b.WriteString(", atas nama " + *sp.PublicCallName)
	}
	b.WriteString(", silakan menuju " + sp.CounterSpoken + ".")
	return b.String()
}
