package domain

import "testing"

func TestDailyKeyDeterministic(t *testing.T) {
	a := DailyKey("secret", "2026-09-12")
	if a != DailyKey("secret", "2026-09-12") {
		t.Fatal("daily key must be deterministic")
	}
	if a == DailyKey("secret", "2026-09-13") {
		t.Error("key must differ across dates")
	}
	if a == DailyKey("other", "2026-09-12") {
		t.Error("key must differ across secrets")
	}
	if len(a) != 16 {
		t.Errorf("key len = %d, want 16", len(a))
	}
}

func TestGenerateSecret(t *testing.T) {
	a, b := GenerateSecret(), GenerateSecret()
	if a == "" || a == b {
		t.Fatalf("secrets must be unique and non-empty: %q", a)
	}
	if len(a) != 64 {
		t.Errorf("secret len = %d, want 64", len(a))
	}
}
