package notify

import (
	"log"
	"strings"

	"github.com/oktaaokta/hostly/internal/domain"
)

var Logger = log.Default()

// Send logs the seat-time notification. Real email/WhatsApp senders replace
// this body later.
func Send(p *domain.Party) {
	var channels []string
	if p.Email != "" {
		channels = append(channels, "email "+p.Email)
	}
	if p.Phone != "" {
		channels = append(channels, "whatsapp "+p.Phone)
	}
	if len(channels) == 0 {
		return
	}
	Logger.Printf("notify: party %d (%s) table ready — via %s — (messaging implementation put here)",
		p.ID, p.Name, strings.Join(channels, ", "))
}
