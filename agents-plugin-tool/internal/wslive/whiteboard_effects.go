package wslive

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

const (
	whiteboardTextMax     = 64 * 1024
	whiteboardEffectsMax  = 128
	whiteboardValueMax    = 4096
	whiteboardTopicMax    = 512
	whiteboardIdentityMax = 32
)

type whiteboardEnvelope struct {
	Version int                   `json:"v"`
	Effects []wswhiteboard.Effect `json:"effects"`
}

// NewWhiteboardEffects projects committed effects into existing ring fields.
// It promises neither append success nor replay; the board remains authoritative.
// Only an empty semantic result omits the entry. Crop values before encoding,
// never serialized JSON, so even a hostile panel remains a complete envelope.
func NewWhiteboardEffects(effects []wswhiteboard.Effect) (RingEntry, bool) {
	if len(effects) == 0 {
		return RingEntry{}, false
	}
	entry := RingEntry{Kind: RingWhiteboardEffects, Caption: fmt.Sprintf("Whiteboard update · %d changes", len(effects))}
	env := whiteboardEnvelope{Version: 1}
	for _, effect := range effects[:min(len(effects), whiteboardEffectsMax)] {
		crop := func(s string, n int) string {
			out := whiteboardCrop(s, n)
			entry.Truncated = entry.Truncated || out != s
			return out
		}
		effect.Kind = crop(effect.Kind, 32)
		effect.Field = crop(effect.Field, 32)
		effect.Topic = crop(effect.Topic, whiteboardTopicMax)
		effect.EntryID = crop(effect.EntryID, whiteboardIdentityMax)
		effect.PanelID = crop(effect.PanelID, whiteboardIdentityMax)
		effect.Label = crop(effect.Label, whiteboardValueMax)
		effect.Before = crop(effect.Before, whiteboardValueMax)
		effect.After = crop(effect.After, whiteboardValueMax)
		// These per-string caps put even the first effect's worst-case JSON
		// escaping below 64 KiB, so a nonempty input always retains one.
		env.Effects = append(env.Effects, effect)
		raw, _ := json.Marshal(env)
		if len(raw) > whiteboardTextMax {
			env.Effects = env.Effects[:len(env.Effects)-1]
			break
		}
		entry.Text = string(raw)
	}
	entry.Truncated = entry.Truncated || len(env.Effects) < len(effects)
	return entry, true
}

// whiteboardCrop bounds bytes at a rune boundary, repairing invalid UTF-8.
func whiteboardCrop(s string, n int) string {
	// Bound work as well as output for a malformed oversized stored record.
	if len(s) > n {
		s = s[:n]
		for !utf8.ValidString(s) && len(s) > 0 && !utf8.RuneStart(s[len(s)-1]) {
			s = s[:len(s)-1]
		}
	}
	s = strings.ToValidUTF8(s, "�")
	if len(s) > n {
		s = s[:n]
		for !utf8.ValidString(s) {
			s = s[:len(s)-1]
		}
	}
	return s
}

// DecodeWhiteboardEffects accepts only the closed v1 display contract. Failure
// must be shown as unavailable raw text, never partially interpreted effects.
func DecodeWhiteboardEffects(entry RingEntry) ([]wswhiteboard.Effect, bool) {
	if entry.Kind != RingWhiteboardEffects || len(entry.Text) > whiteboardTextMax || !utf8.ValidString(entry.Text) || len(entry.Caption) > 200*utf8.UTFMax || utf8.RuneCountInString(entry.Caption) > 200 || !utf8.ValidString(entry.Caption) {
		return nil, false
	}
	var env whiteboardEnvelope
	if !whiteboardJSON([]byte(entry.Text), &env) || env.Version != 1 || len(env.Effects) == 0 || len(env.Effects) > whiteboardEffectsMax {
		return nil, false
	}
	for _, e := range env.Effects {
		if !validWhiteboardEffect(e) {
			return nil, false
		}
	}
	return env.Effects, true
}

// whiteboardJSON also rejects duplicate keys and nulls: encoding/json alone
// silently accepts both, which would make an ambiguous carrier look typed.
func whiteboardJSON(raw []byte, out any) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	depth := 0
	var value func() bool
	value = func() bool {
		depth++
		defer func() { depth-- }()
		if depth > 4 {
			return false
		}
		t, err := d.Token()
		if err != nil || t == nil {
			return false
		}
		switch t {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				k, ok := key.(string)
				if err != nil || !ok || seen[k] {
					return false
				}
				allowed := []string{"v", "effects"}
				if depth == 3 {
					allowed = []string{"kind", "topic", "id", "entryId", "panelId", "field", "before", "after", "label"}
				}
				if !slices.Contains(allowed, k) {
					return false
				}
				seen[k] = true
				if !value() {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case json.Delim('['):
			for d.More() {
				if !value() {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		}
		return true
	}
	if !value() {
		return false
	}
	if _, err := d.Token(); err != io.EOF {
		return false
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(out) == nil
}

func validWhiteboardEffect(e wswhiteboard.Effect) bool {
	if !slices.Contains(wswhiteboard.EffectFields(e.Kind), e.Field) || len(e.Topic) > whiteboardTopicMax || len(e.Label) > whiteboardValueMax || len(e.Before) > whiteboardValueMax || len(e.After) > whiteboardValueMax {
		return false
	}
	if e.Field == "presence" && (e.Kind == "item" || e.Kind == "topic") {
		if !slices.Contains([]string{"", "present"}, e.Before) || !slices.Contains([]string{"", "present"}, e.After) || e.Before == e.After {
			return false
		}
	}
	if e.Kind == "topic" && e.Field == "closed" {
		if !slices.Contains([]string{"", "closed"}, e.Before) || !slices.Contains([]string{"", "closed"}, e.After) || e.Before == e.After {
			return false
		}
	}
	if e.Kind == "setting" && e.Field == "id-floor" {
		for _, s := range []string{e.Before, e.After} {
			n, err := strconv.ParseUint(s, 10, 53)
			if err != nil || strconv.FormatUint(n, 10) != s {
				return false
			}
		}
	}
	id := func(s, prefix string) bool {
		if len(s) < 2 || len(s) > whiteboardIdentityMax || !strings.HasPrefix(s, prefix) || s[1] == '0' {
			return false
		}
		n, err := strconv.ParseUint(s[1:], 10, 53)
		return err == nil && n > 0 && prefix+strconv.FormatUint(n, 10) == s
	}
	switch e.Kind {
	case "item":
		return e.ID > 0 && int64(e.ID) <= 1<<53-1 && e.EntryID == "" && e.PanelID == ""
	case "topic":
		return e.Topic != "" && e.ID == 0 && e.EntryID == "" && e.PanelID == ""
	case "panel":
		return e.Topic != "" && e.ID == 0 && id(e.EntryID, "e") && id(e.PanelID, "p")
	case "log":
		return e.Topic != "" && e.ID == 0 && id(e.EntryID, "e") && e.PanelID == ""
	case "setting":
		return e.Topic == "" && e.ID == 0 && e.EntryID == "" && e.PanelID == ""
	}
	return false
}

// WhiteboardEffectsFallback is bounded plain text for an unreadable carrier.
// The caller must escape it, just like valid before/after values.
func WhiteboardEffectsFallback(entry RingEntry) string {
	return whiteboardCrop(entry.Text, whiteboardValueMax)
}
