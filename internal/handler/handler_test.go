package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/repository"
	"github.com/oktaaokta/hostly/internal/usecase"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := repository.NewMemory()
	ven := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok", DailySecret: "testsecret"}
	if err := m.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	q := usecase.NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) })
	h := New(q, NewHub(), "")
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts
}

func joinQuery() string {
	date := joinQueryDate()
	return "?d=" + date + "&k=" + domain.DailyKey("testsecret", date)
}

func joinQueryDate() string {
	return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func post(t *testing.T, ts *httptest.Server, path string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	resp, err := http.Post(ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestJoinEndpoint(t *testing.T) {
	ts := newTestServer(t)

	resp := post(t, ts, "/api/venues/joes/parties"+joinQuery(), map[string]any{"name": "Alex", "pax": 2, "email": "alex@x.com"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got struct {
		Ahead int `json:"ahead"`
	}
	json.NewDecoder(resp.Body).Decode(&got)
	resp.Body.Close()
	if got.Ahead != 0 {
		t.Errorf("ahead = %v", got.Ahead)
	}

	resp = post(t, ts, "/api/venues/joes/parties"+joinQuery(), map[string]any{"name": "Alex", "pax": 2, "email": "alex@x.com"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = post(t, ts, "/api/venues/joes/parties"+joinQuery(), map[string]any{"name": "", "pax": 2, "email": "alex@x.com"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("empty-name status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStaffAuthRequired(t *testing.T) {
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/venues/joes/staff")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("missing token status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = http.Get(ts.URL + "/api/venues/joes/staff?token=bad")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("bad token status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestStaffSeatFlow(t *testing.T) {
	ts := newTestServer(t)
	join := post(t, ts, "/api/venues/joes/parties"+joinQuery(), map[string]any{"name": "Bea", "pax": 4, "email": "bea@x.com"})
	var jr struct {
		Party struct {
			ID int64 `json:"id"`
		} `json:"party"`
	}
	json.NewDecoder(join.Body).Decode(&jr)
	join.Body.Close()

	path := fmt.Sprintf("/api/venues/joes/parties/%d/seat?token=tok", jr.Party.ID)
	resp, err := http.Post(ts.URL+path, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("seat status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	get, err := http.Get(ts.URL + "/api/venues/joes")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var cv struct {
		IsOpen       bool `json:"is_open"`
		WaitingCount int  `json:"waiting_count"`
	}
	json.NewDecoder(get.Body).Decode(&cv)
	if !cv.IsOpen || cv.WaitingCount != 0 {
		t.Errorf("customer view after seat = %+v", cv)
	}
}

func TestJoinDailyGate(t *testing.T) {
	ts := newTestServer(t)

	resp := post(t, ts, "/api/venues/joes/parties?d="+joinQueryDate()+"&k=deadbeef", map[string]any{"name": "A", "pax": 1, "email": "a@x.com"})
	if resp.StatusCode != http.StatusGone {
		t.Errorf("bad key status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = post(t, ts, "/api/venues/joes/parties?d=2000-01-01&k="+domain.DailyKey("testsecret", "2000-01-01"), map[string]any{"name": "B", "pax": 1, "email": "b@x.com"})
	if resp.StatusCode != http.StatusGone {
		t.Errorf("old date status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = post(t, ts, "/api/venues/joes/parties"+joinQuery(), map[string]any{"name": "C", "pax": 1})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("no contact status = %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestQRPNGEndpoint(t *testing.T) {
	ts := newTestServer(t)

	date := joinQueryDate()
	resp, err := http.Get(ts.URL + "/api/venues/joes/qr.png?d=" + date + "&k=" + domain.DailyKey("testsecret", date))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
		t.Fatalf("status=%d type=%q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	b, _ := io.ReadAll(resp.Body)
	if len(b) < 8 || !bytes.Equal(b[:4], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatalf("not a png, len=%d", len(b))
	}

	resp2, err := http.Get(ts.URL + "/api/venues/joes/qr.png?d=" + date + "&k=deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusGone {
		t.Errorf("stale qr status = %d", resp2.StatusCode)
	}
}

func TestRotateQR(t *testing.T) {
	ts := newTestServer(t)

	date := joinQueryDate()
	path := "/api/venues/joes/staff/rotate-qr?token=tok"
	resp, err := http.Post(ts.URL+path, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rotate status = %d", resp.StatusCode)
	}

	resp, err = http.Get(ts.URL + "/api/venues/joes/qr.png?d=" + date + "&k=" + domain.DailyKey("testsecret", date))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusGone {
		t.Errorf("pre-rotate key still alive: %d", resp.StatusCode)
	}

	get, err := http.Get(ts.URL + "/api/venues/joes/staff?token=tok")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var sv struct {
		QRCode struct {
			Link  string `json:"link"`
			QRURL string `json:"qr_url"`
		} `json:"qrcode"`
	}
	json.NewDecoder(get.Body).Decode(&sv)
	if !strings.HasPrefix(sv.QRCode.QRURL, "http") || !strings.Contains(sv.QRCode.QRURL, "/api/venues/joes/qr.png?d=") {
		t.Errorf("qr_url = %q", sv.QRCode.QRURL)
	}
	if !strings.HasPrefix(sv.QRCode.Link, "/api/venues/joes/qr.png?d=") {
		t.Errorf("link = %q", sv.QRCode.Link)
	}
}

func TestWebSocketBroadcast(t *testing.T) {
	ts := newTestServer(t)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/venues/joes/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	join := post(t, ts, "/api/venues/joes/parties"+joinQuery(), map[string]any{"name": "Wes", "pax": 2, "email": "wes@x.com"})
	if join.StatusCode != http.StatusCreated {
		t.Fatalf("join status = %d", join.StatusCode)
	}
	var jr struct {
		Party struct {
			ID int64 `json:"id"`
		} `json:"party"`
	}
	json.NewDecoder(join.Body).Decode(&jr)
	join.Body.Close()

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ws read: %v", err)
	}
	var ev struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Type != "party_joined" {
		t.Errorf("ws type = %q, want party_joined", ev.Type)
	}

	conn.Close()

	path := fmt.Sprintf("/api/venues/joes/parties/%d/seat?token=tok", jr.Party.ID)
	seatResp, err := http.Post(ts.URL+path, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer seatResp.Body.Close()
	if seatResp.StatusCode != http.StatusOK {
		t.Errorf("seat status after ws close = %d", seatResp.StatusCode)
	}
}
