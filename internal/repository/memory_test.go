package repository

import (
	"testing"

	"github.com/oktaaokta/hostly/internal/domain"
)

func TestMemoryAutoSecret(t *testing.T) {
	m := NewMemory()
	v := &domain.Venue{Slug: "a", Name: "A", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "t"}
	if err := m.Venues().Create(v); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Venues().GetBySlug("a")
	if len(got.DailySecret) != 64 {
		t.Errorf("auto secret len = %d, want 64", len(got.DailySecret))
	}
}
