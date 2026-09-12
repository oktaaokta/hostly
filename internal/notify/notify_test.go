package notify

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/oktaaokta/hostly/internal/domain"
)

func TestSendLogsChannels(t *testing.T) {
	var buf bytes.Buffer
	Logger = log.New(&buf, "", 0)
	p := &domain.Party{ID: 5, Name: "Rita", Email: "rita@x.com", Phone: "+62812345678"}
	Send(p)
	out := buf.String()
	for _, want := range []string{"party 5", "Rita", "email rita@x.com", "whatsapp +62812345678", "(messaging implementation put here)"} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q: %s", want, out)
		}
	}
}

func TestSendNoContactSilent(t *testing.T) {
	var buf bytes.Buffer
	Logger = log.New(&buf, "", 0)
	Send(&domain.Party{ID: 1, Name: "Nobody"})
	if buf.Len() != 0 {
		t.Errorf("expected no log, got %q", buf.String())
	}
}
