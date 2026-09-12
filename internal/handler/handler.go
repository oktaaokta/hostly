package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/oktaaokta/hostly/internal/domain"
	"github.com/oktaaokta/hostly/internal/usecase"
)

type Handler struct {
	us       *usecase.Queue
	hub      *Hub
	upgrader websocket.Upgrader
}

func New(us *usecase.Queue, hub *Hub) *Handler {
	return &Handler{
		us:       us,
		hub:      hub,
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
	}
}

type event struct {
	Type  string `json:"type"`
	Party any    `json:"party,omitempty"`
	Venue any    `json:"venue,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalid):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrUnauthorized):
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case errors.Is(err, domain.ErrClosed), errors.Is(err, domain.ErrDuplicate), errors.Is(err, domain.ErrNotWaiting):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Get("/api/venues/{slug}", h.getCustomerView)
	r.Post("/api/venues/{slug}/parties", h.join)
	r.Get("/api/venues/{slug}/staff", h.getStaffView)
	r.Patch("/api/venues/{slug}/parties/{id}", h.editParty)
	r.Post("/api/venues/{slug}/parties/{id}/seat", h.seat)
	r.Post("/api/venues/{slug}/parties/{id}/leave", h.leave)
	r.Post("/api/venues/{slug}/parties/{id}/top", h.top)
	r.Patch("/api/venues/{slug}/hours", h.updateHours)
	r.Get("/api/venues/{slug}/ws", h.websocket)
}

func (h *Handler) getCustomerView(w http.ResponseWriter, r *http.Request) {
	v, err := h.us.CustomerView(chi.URLParam(r, "slug"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

type joinRequest struct {
	Name  string `json:"name"`
	Pax   int    `json:"pax"`
	Note  string `json:"note"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var body joinRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	res, err := h.us.Join(slug, body.Name, body.Pax, body.Note, body.Email, body.Phone,
		r.URL.Query().Get("k"), r.URL.Query().Get("d"))
	if err != nil {
		writeErr(w, err)
		return
	}
	ven, err := h.us.VenueBySlug(slug)
	if err == nil {
		h.hub.Broadcast(ven.ID, event{Type: "party_joined", Party: res.Party})
	}
	writeJSON(w, http.StatusCreated, res)
}

func token(r *http.Request) string {
	if t := r.URL.Query().Get("token"); t != "" {
		return t
	}
	return r.Header.Get("X-Staff-Token")
}

func (h *Handler) getStaffView(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	v, err := h.us.StaffView(slug, token(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

// partyAction runs a staff action on a party and broadcasts a venue update.
func (h *Handler) partyAction(w http.ResponseWriter, r *http.Request, act func(int64) error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	if err := act(id); err != nil {
		writeErr(w, err)
		return
	}
	h.broadcastVenue(r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) seat(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.partyAction(w, r, func(id int64) error { return h.us.Seat(slug, token(r), id) })
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.partyAction(w, r, func(id int64) error { return h.us.Leave(slug, token(r), id) })
}

func (h *Handler) top(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	h.partyAction(w, r, func(id int64) error { return h.us.Top(slug, token(r), id) })
}

type editRequest struct {
	Pax  int    `json:"pax"`
	Note string `json:"note"`
}

func (h *Handler) editParty(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	var body editRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	if err := h.us.EditParty(slug, token(r), id, body.Pax, body.Note); err != nil {
		writeErr(w, err)
		return
	}
	h.broadcastVenue(r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type hoursRequest struct {
	OpenTime  string  `json:"open_time"`
	CloseTime string  `json:"close_time"`
	Override  *string `json:"override"`
}

func (h *Handler) updateHours(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	var body hoursRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, domain.ErrInvalid)
		return
	}
	if err := h.us.UpdateHours(slug, token(r), body.OpenTime, body.CloseTime, body.Override); err != nil {
		writeErr(w, err)
		return
	}
	h.broadcastVenue(r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// broadcastVenue pushes the current customer view to every client of the venue.
func (h *Handler) broadcastVenue(r *http.Request) {
	slug := chi.URLParam(r, "slug")
	v, err := h.us.CustomerView(slug)
	if err != nil {
		return
	}
	h.hub.Broadcast(v.Venue.ID, event{Type: "venue_updated", Venue: v})
}

func (h *Handler) websocket(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	ven, err := h.us.VenueBySlug(slug)
	if err != nil {
		writeErr(w, err)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.hub.add(ven.ID, conn)
	defer h.hub.remove(ven.ID, conn)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
