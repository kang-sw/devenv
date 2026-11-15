package wsagent

import (
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsinbox"
)

// expectedUserEnvelope pins the wire grammar independently of Entry.Text.
func expectedUserEnvelope(at time.Time, text string) string {
	return `<ws:user at="` + at.In(time.Local).Format("2006-01-02T15:04-07:00") + `">` + "\n" + text + "\n</ws:user>"
}

// Expectations use the harness's frozen clock and the independent wire grammar
// in expectedUserEnvelope. Each argument is one inbox entry, not a whole batch:
// merged turns must explicitly join separately wrapped entries in queue order.
func userEnvelopes(now time.Time, texts ...string) []string {
	out := make([]string, len(texts))
	for i, text := range texts {
		out[i] = expectedUserEnvelope(now, text)
	}
	return out
}

// The dashboard's page route has its own real clock, outside Runner.Options.Now.
// Check its complete envelope and exact body without a minute-boundary race.
func assertUserEnvelopeText(t *testing.T, got, want string) {
	t.Helper()
	messages, ok := wsinbox.ParseUserMessages(got)
	if !ok || len(messages) != 1 || messages[0].At == "" || messages[0].Text != want {
		t.Fatalf("user envelope = %q, want one envelope containing %q", got, want)
	}
	at, err := time.Parse("2006-01-02T15:04-07:00", messages[0].At)
	if err != nil || got != expectedUserEnvelope(at, want) {
		t.Fatalf("noncanonical user envelope = %q", got)
	}
}

func (h *harness) envelope(text string) string {
	return expectedUserEnvelope(h.config.Now(), text)
}

func (h *leadHarness) envelope(text string) string {
	return expectedUserEnvelope(h.opts.Now(), text)
}

func (h *leadHarness) envelopes(texts ...string) []string {
	return userEnvelopes(h.opts.Now(), texts...)
}
