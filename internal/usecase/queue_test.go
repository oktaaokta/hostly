package usecase

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/repository"
)

var testNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func testKey(ven *domain.Venue) string {
	return domain.DailyKey(ven.DailySecret, testNow.Format("2006-01-02"))
}

func openVenue(t *testing.T, m *repository.Memory) *domain.Venue {
	t.Helper()
	v := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok", DailySecret: domain.GenerateSecret()}
	if err := m.Venues().Create(v); err != nil {
		t.Fatal(err)
	}
	return v
}

func seedG(t *testing.T, m *repository.Memory) *domain.Venue {
	t.Helper()
	v := &domain.Venue{Slug: "g", Name: "G", OpenTime: "00:00", CloseTime: "23:59", StaffToken: "t"}
	if err := m.Venues().Create(v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestJoinHappyPath(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })
	date := testNow.Format("2006-01-02")

	res, err := q.Join("joes", "Alex", 2, "", "a@b.co", "", testKey(ven), date)
	if err != nil {
		t.Fatal(err)
	}
	if res.Ahead != 0 {
		t.Errorf("first party ahead = %d, want 0", res.Ahead)
	}
	if res.Party.Status != domain.PartyWaiting {
		t.Errorf("status = %v", res.Party.Status)
	}

	res2, err := q.Join("joes", "Bea", 4, "booth", "a@b.co", "", testKey(ven), date)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Ahead != 1 {
		t.Errorf("second party ahead = %d, want 1", res2.Ahead)
	}
	if !(res2.Party.Order > res.Party.Order) {
		t.Error("later party should have higher order")
	}
}

func TestJoinValidation(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), time.Now)
	today := time.Now().Format("2006-01-02")
	key := domain.DailyKey(ven.DailySecret, today)

	if _, err := q.Join("joes", "  ", 2, "", "a@b.co", "", key, today); err != domain.ErrInvalid {
		t.Errorf("empty name err = %v", err)
	}
	if _, err := q.Join("joes", "Alex", 0, "", "a@b.co", "", key, today); err != domain.ErrInvalid {
		t.Errorf("pax 0 err = %v", err)
	}
	if _, err := q.Join("joes", "Alex", 21, "", "a@b.co", "", key, today); err != domain.ErrInvalid {
		t.Errorf("pax 21 err = %v", err)
	}
	if _, err := q.Join("missing", "Alex", 2, "", "a@b.co", "", key, today); err != domain.ErrNotFound {
		t.Errorf("missing venue err = %v", err)
	}
}

func TestJoinDuplicate(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })
	date := testNow.Format("2006-01-02")

	if _, err := q.Join("joes", "Alex", 2, "", "a@b.co", "", testKey(ven), date); err != nil {
		t.Fatal(err)
	}
	if _, err := q.Join("joes", "alex", 3, "", "a@b.co", "", testKey(ven), date); err != domain.ErrDuplicate {
		t.Errorf("duplicate name err = %v", err)
	}
}

func TestJoinWhenClosed(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 23, 0, 0, 0, time.UTC) })

	if _, err := q.Join("joes", "Alex", 2, "", "a@b.co", "", testKey(ven), testNow.Format("2006-01-02")); err != domain.ErrClosed {
		t.Errorf("closed venue err = %v", err)
	}
}

func mustJoin(t *testing.T, q *Queue, ven *domain.Venue, name string, pax int) *domain.Party {
	t.Helper()
	res, err := q.Join(ven.Slug, name, pax, "", "a@b.co", "", testKey(ven), testNow.Format("2006-01-02"))
	if err != nil {
		t.Fatal(err)
	}
	return res.Party
}

func TestSeatLeaveTopEditHours(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })

	a := mustJoin(t, q, ven, "Alex", 2)
	b := mustJoin(t, q, ven, "Bea", 4)

	if err := q.Seat(ven.Slug, "wrong", a.ID); err != domain.ErrUnauthorized {
		t.Errorf("bad token err = %v", err)
	}

	if err := q.Seat(ven.Slug, "tok", a.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Seat(ven.Slug, "tok", a.ID); err != domain.ErrNotWaiting {
		t.Errorf("re-seat err = %v", err)
	}

	view, _ := q.StaffView(ven.Slug, "tok")
	if view.Stats.Waiting != 1 || view.Stats.SeatedToday != 1 {
		t.Errorf("stats after seat = %+v", view.Stats)
	}
	firstWaiting := int64(-1)
	for _, p := range view.Parties {
		if p.Status == domain.PartyWaiting {
			firstWaiting = p.ID
			break
		}
	}
	if firstWaiting != b.ID {
		t.Errorf("front of queue after seat = %d, want %d", firstWaiting, b.ID)
	}

	if err := q.Leave(ven.Slug, "tok", b.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.Leave(ven.Slug, "tok", b.ID); err != domain.ErrNotWaiting {
		t.Errorf("double leave err = %v", err)
	}

	c := mustJoin(t, q, ven, "Cid", 2)
	if err := q.Top(ven.Slug, "tok", c.ID); err != nil {
		t.Fatal(err)
	}
	view2, _ := q.CustomerView(ven.Slug)
	if len(view2.Waiting) != 1 || view2.Waiting[0].ID != c.ID {
		t.Errorf("after top, front = %+v", view2.Waiting)
	}
	if view2.WaitingCount != 1 {
		t.Errorf("waiting count = %d", view2.WaitingCount)
	}

	if err := q.EditParty(ven.Slug, "tok", c.ID, 5, "window seat"); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateHours(ven.Slug, "tok", "11:00", "23:00", nil); err != nil {
		t.Fatal(err)
	}
	info, _ := q.CustomerView(ven.Slug)
	if info.Venue.OpenTime != "11:00" {
		t.Errorf("open time = %s", info.Venue.OpenTime)
	}
	if info.Waiting[0].Pax != 5 {
		t.Errorf("edited pax = %d", info.Waiting[0].Pax)
	}

	if _, err := q.StaffView(ven.Slug, "nope"); err != domain.ErrUnauthorized {
		t.Errorf("staff bad token err = %v", err)
	}
	if _, err := q.StaffView("missing", "tok"); err != domain.ErrNotFound {
		t.Errorf("staff missing venue err = %v", err)
	}
}

func TestJSONContractSnakeCaseRedactsToken(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })
	date := testNow.Format("2006-01-02")

	res, err := q.Join(ven.Slug, "Alex", 2, "", "a@b.co", "", testKey(ven), date)
	if err != nil {
		t.Fatal(err)
	}

	asMap := func(t *testing.T, b []byte) map[string]any {
		t.Helper()
		var out map[string]any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatalf("unmarshal: %v\n%s", err, b)
		}
		return out
	}

	jr, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	jm := asMap(t, jr)
	if jm["ahead"] == nil || jm["party"] == nil {
		t.Errorf("JoinResult keys = %v", jm)
	}

	cv, err := q.CustomerView(ven.Slug)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := json.Marshal(cv)
	if err != nil {
		t.Fatal(err)
	}
	cm := asMap(t, cb)
	venueJSON, _ := cm["venue"].(map[string]any)
	if venueJSON["open_time"] != "10:00" {
		t.Errorf("venue open_time = %v (%s)", venueJSON["open_time"], cb)
	}
	if _, ok := venueJSON["staff_token"]; ok {
		t.Errorf("venue leaks staff_token: %s", cb)
	}
	if _, ok := venueJSON["StaffToken"]; ok {
		t.Errorf("venue leaks StaffToken: %s", cb)
	}
	if _, ok := venueJSON["daily_secret"]; ok {
		t.Errorf("venue leaks daily_secret: %s", cb)
	}

	sv, err := q.StaffView(ven.Slug, "tok")
	if err != nil {
		t.Fatal(err)
	}
	sb, err := json.Marshal(sv)
	if err != nil {
		t.Fatal(err)
	}
	sm := asMap(t, sb)
	if _, ok := sm["staff_token"]; ok {
		t.Errorf("staff view leaks staff_token: %s", sb)
	}
	parties, _ := sm["parties"].([]any)
	if len(parties) != 1 {
		t.Fatalf("staff parties = %v", parties)
	}
	pjson, _ := parties[0].(map[string]any)
	if pjson["created_at"] == nil || pjson["venue_id"] == nil {
		t.Errorf("party JSON not snake_case: %s", sb)
	}
}

func TestJoinRequiresValidKeySameDay(t *testing.T) {
	m := repository.NewMemory()
	h := NewQueue(m.Venues(), m.Parties(), time.Now)
	v := seedG(t, m)
	today := time.Now().Format("2006-01-02")
	good := domain.DailyKey(v.DailySecret, today)
	if _, err := h.Join("g", "A", 1, "", "a@b.co", "", good, today); err != nil {
		t.Fatalf("valid same-day key rejected: %v", err)
	}
	if _, err := h.Join("g", "B", 1, "", "a@b.co", "", "deadbeef", today); err != nil {
		if !errors.Is(err, domain.ErrStale) {
			t.Fatalf("wrong key err = %v, want ErrStale", err)
		}
	} else {
		t.Fatal("wrong key accepted")
	}
	if _, err := h.Join("g", "C", 1, "", "a@b.co", "", good, "2000-01-01"); err != nil {
		if !errors.Is(err, domain.ErrStale) {
			t.Fatalf("yesterday key err = %v, want ErrStale", err)
		}
	} else {
		t.Fatal("yesterday key accepted")
	}
}

func TestJoinUpdatedValidations(t *testing.T) {
	m := repository.NewMemory()
	h := NewQueue(m.Venues(), m.Parties(), time.Now)
	v := seedG(t, m)
	today := time.Now().Format("2006-01-02")
	key := domain.DailyKey(v.DailySecret, today)
	noContact := []ct{{"", ""}, {"a@b.co", "+1"}}
	for i, c := range noContact {
		if _, err := h.Join("g", "B", 1, "", c.email, c.phone, key, today); err == nil {
			t.Fatalf("case %d: no contact accepted", i)
		}
	}
	if _, err := h.Join("g", "B", 1, "", "bad", "", key, today); err == nil {
		t.Fatal("invalid email accepted")
	}
	if _, err := h.Join("g", "B", 1, "", "", "123", key, today); err == nil {
		t.Fatal("invalid phone accepted")
	}
}

type ct struct{ email, phone string }

func TestRotateSecretInvalidatesKey(t *testing.T) {
	m := repository.NewMemory()
	h := NewQueue(m.Venues(), m.Parties(), time.Now)
	v := seedG(t, m)
	today := time.Now().Format("2006-01-02")
	if err := h.RotateSecret("g", "t"); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Venues().GetBySlug("g")
	if got.DailySecret == v.DailySecret {
		t.Fatal("secret did not change")
	}
	if _, err := h.Join("g", "A", 1, "", "a@b.co", "", domain.DailyKey(v.DailySecret, today), today); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("rotated secret still accepted: %v", err)
	}
}

func TestQRPNGValidPng(t *testing.T) {
	m := repository.NewMemory()
	h := NewQueue(m.Venues(), m.Parties(), time.Now)
	v := seedG(t, m)
	today := time.Now().Format("2006-01-02")
	k := domain.DailyKey(v.DailySecret, today)
	png, err := h.QRPNG("g", k, today, "http://host")
	if err != nil {
		t.Fatal(err)
	}
	if len(png) < 100 || png[0] != 0x89 || png[1] != 'P' || png[2] != 'N' || png[3] != 'G' {
		t.Fatalf("not a png, len=%d", len(png))
	}
	if _, err := h.QRPNG("g", "deadbeef", today, "http://host"); !errors.Is(err, domain.ErrStale) {
		t.Fatalf("bad key err = %v, want ErrStale", err)
	}
}

func TestSeatNotifiesAndStamps(t *testing.T) {
	m := repository.NewMemory()
	ven := openVenue(t, m)
	q := NewQueue(m.Venues(), m.Parties(), func() time.Time { return testNow })

	withContact := mustJoin(t, q, ven, "Rita", 2)
	if err := q.Seat(ven.Slug, "tok", withContact.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := m.Parties().Get(withContact.ID)
	if got.NotifiedAt == nil || !got.NotifiedAt.Equal(testNow) {
		t.Errorf("notified_at = %v", got.NotifiedAt)
	}

	noContact := mustJoin(t, q, ven, "Bob", 3)
	noContact.Email, noContact.Phone = "", ""
	if err := m.Parties().Update(noContact); err != nil {
		t.Fatal(err)
	}
	if err := q.Seat(ven.Slug, "tok", noContact.ID); err != nil {
		t.Fatal(err)
	}
	got2, _ := m.Parties().Get(noContact.ID)
	if got2.NotifiedAt != nil {
		t.Errorf("no-contact notified_at = %v", got2.NotifiedAt)
	}
}

func TestSeatBuildsQRCodeInfo(t *testing.T) {
	m := repository.NewMemory()
	h := NewQueue(m.Venues(), m.Parties(), time.Now)
	v := seedG(t, m)
	today := time.Now().Format("2006-01-02")
	k := domain.DailyKey(v.DailySecret, today)
	if _, err := h.Join("g", "A", 1, "", "a@b.co", "", k, today); err != nil {
		t.Fatal(err)
	}
	sv, err := h.StaffView("g", "t")
	if err != nil {
		t.Fatal(err)
	}
	if sv.QRCode.Link == "" || !strings.HasPrefix(sv.QRCode.Link, "/api/venues/g/qr.png?") {
		t.Fatalf("qrcode link = %q", sv.QRCode.Link)
	}
	if sv.QRCode.Link == "" {
		t.Fatal("empty qrcode")
	}
}
