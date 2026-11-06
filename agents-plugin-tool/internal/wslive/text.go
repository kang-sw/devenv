package wslive

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wsmailbox"
)

// cleanLine makes s one display line: control characters dropped, whitespace
// runs collapsed to one space, trimmed, and capped at maxRunes with a
// trailing ellipsis.
func cleanLine(s string, maxRunes int) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			space = true
			continue
		case unicode.IsControl(r):
			continue
		}
		if space && b.Len() > 0 {
			b.WriteByte(' ')
		}
		space = false
		b.WriteRune(r)
	}
	out := b.String()
	if utf8.RuneCountInString(out) <= maxRunes {
		return out
	}
	if maxRunes <= 1 {
		return "…"
	}
	runes := []rune(out)
	return string(runes[:maxRunes-1]) + "…"
}

// cleanToolName is a tool name as the snapshot shows it.
func cleanToolName(name string) string {
	if out := cleanLine(name, ToolNameRunes); out != "" {
		return out
	}
	return "tool"
}

// flattenArgs is a tool call's arguments as preview text: object entries as
// "key: value" in key order (strings raw, everything else JSON). The caller
// scrubs it before cleanLine caps it, so the cap never cuts a key in half.
func flattenArgs(args any) string {
	var text string
	switch v := args.(type) {
	case nil:
	case string:
		text = v
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			value, ok := v[k].(string)
			if !ok {
				value = jsonText(v[k])
			}
			parts = append(parts, k+": "+value)
		}
		text = strings.Join(parts, ", ")
	default:
		text = jsonText(v)
	}
	return text
}

func jsonText(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

// capText bounds a multi-line text at TextBytes on a rune boundary, keeping
// its head.
func capText(s string) (string, bool) {
	if len(s) <= TextBytes {
		return s, false
	}
	cut := TextBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut], true
}

// sessionKeyArg matches a session key passed as a named argument in a
// flattened preview or a JSON text ("session_key: <key>",
// `"sessionKey":"<key>"`), so the key can be replaced by its reply-id.
var sessionKeyArg = regexp.MustCompile(`(?i)(session_?key["']?\s*[:=]\s*["']?)([a-z0-9][a-z0-9_-]*)`)

// scrubber replaces session keys in browser-bound text with the session's
// reply-id ("id:<reply-id>"): every key it knows (the session's own and
// every delegate key an adapter reported), and any value passed as a named
// session-key argument. Tool argument previews carry the lead's own key on
// every ws tool call, so the snapshot never stores a preview unscrubbed.
type scrubber struct {
	keys []string
}

// scrubberKeyCap bounds the remembered delegate keys; the oldest is
// forgotten first. A forgotten key is still caught as a named argument.
const scrubberKeyCap = 256

func (s *scrubber) add(key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	for _, k := range s.keys {
		if k == key {
			return
		}
	}
	s.keys = append(s.keys, key)
	if len(s.keys) > scrubberKeyCap {
		// Keep the session's own key (the first) and drop the oldest delegate.
		s.keys = append(s.keys[:1], s.keys[2:]...)
	}
}

func (s *scrubber) scrub(text string) string {
	if text == "" {
		return text
	}
	text = sessionKeyArg.ReplaceAllStringFunc(text, func(m string) string {
		sub := sessionKeyArg.FindStringSubmatch(m)
		if strings.EqualFold(sub[2], "id") {
			return m // already a reply-id handle ("id:<reply-id>")
		}
		return sub[1] + replyHandle(sub[2])
	})
	for _, key := range s.keys {
		if strings.Contains(text, key) {
			text = strings.ReplaceAll(text, key, replyHandle(key))
		}
	}
	return text
}

func replyHandle(key string) string {
	return wsmailbox.ReplyIDPrefix + wsmailbox.ReplyID(key)
}
