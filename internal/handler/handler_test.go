package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/repository"
	"github.com/oktaaokta/hostly/internal/usecase"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	m := repository.NewMemory()
	ven := &domain.Venue{Slug: "joes", Name: "Joe's", OpenTime: "10:00", CloseTime: "22:00", StaffToken: "tok"}
	if err := m.Venues().Create(ven); err != nil {
		t.Fatal(err)
	}
	q := usecase.NewQueue(m.Venues(), m.Parties(), func() time.Time { return time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC) })
	h := New(q, NewHub())
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	ts := httptest.NewServer(r)
	t.Cleanup(ts.Close)
	return ts
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

	resp := post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "Alex", "pax": 2})
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

	resp = post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "Alex", "pax": 2})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "", "pax": 2})
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
	join := post(t, ts, "/api/venues/joes/parties", map[string]any{"name": "Bea", "pax": 4})
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
