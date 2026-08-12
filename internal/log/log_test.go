package log

import "testing"

func TestMaskPhone(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"5511999999999", "55****9999"},
		{"5511999999999@s.whatsapp.net", "55****9999"},
		{"+5511999999999", "55****9999"},
		{"11999999999", "11****9999"},
		{"abc", "****"},
		{"", ""},
	}
	for _, c := range cases {
		got := MaskPhone(c.in)
		if got != c.want {
			t.Errorf("MaskPhone(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTruncName(t *testing.T) {
	if TruncName("") != "" {
		t.Error("empty should return empty")
	}
	if TruncName("Curto") != "Curto" {
		t.Error("short names should pass through")
	}
	long := "João da Silva Pereira de Souza e Albuquerque Neto"
	out := TruncName(long)
	if len(out) > 40 {
		t.Errorf("expected truncated (≤40), got %d: %q", len(out), out)
	}
	// Mesma string → mesmo hash de 6 chars
	if TruncName(long) != out {
		t.Error("truncation should be deterministic")
	}
}

func TestContentHashIsDeterministic(t *testing.T) {
	a := ContentHash("hello")
	b := ContentHash("hello")
	if a != b {
		t.Error("same content should hash to same value")
	}
	if ContentHash("hello") == ContentHash("hello!") {
		t.Error("different content should hash to different value")
	}
	if len(a) != 64 { // sha256 hex = 64 chars
		t.Errorf("expected 64 hex chars, got %d", len(a))
	}
}
