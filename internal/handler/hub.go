package handler

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// Hub fans JSON events out to every connected client for a venue. Rooms are
// keyed by venue ID. Events are change signals: clients refetch state over
// REST after receiving one.
type Hub struct {
	mu    sync.Mutex
	rooms map[int64]map[*websocket.Conn]struct{}
}

func NewHub() *Hub {
	return &Hub{rooms: map[int64]map[*websocket.Conn]struct{}{}}
}

func (h *Hub) add(venueID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[venueID] == nil {
		h.rooms[venueID] = map[*websocket.Conn]struct{}{}
	}
	h.rooms[venueID][c] = struct{}{}
}

func (h *Hub) remove(venueID int64, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[venueID]
	if room == nil {
		return
	}
	delete(room, c)
	if len(room) == 0 {
		delete(h.rooms, venueID)
	}
}

// Broadcast marshals ev and writes it to every connection in the room.
func (h *Hub) Broadcast(venueID int64, ev any) {
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[venueID] {
		if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
			c.Close()
			delete(h.rooms[venueID], c)
		}
	}
}
