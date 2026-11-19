package wslive_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/kang-sw/devenv/internal/wslive"
	oldring "github.com/kang-sw/devenv/internal/wslive/testdata/pre_effects"
	"github.com/kang-sw/devenv/internal/wswhiteboard"
)

func TestWhiteboardEffectsBoundsAndShape(t *testing.T) {
	if _, ok := wslive.NewWhiteboardEffects(nil); ok {
		t.Fatal("empty effects emitted")
	}
	for _, value := range []string{strings.Repeat("雪🙂", 30000), strings.Repeat("<>&\x00\"\\", 30000), "invalid\xff\xfe", strings.Repeat("a", 4095) + "雪"} {
		effects := make([]wswhiteboard.Effect, 300)
		for i := range effects {
			effects[i] = wswhiteboard.Effect{Kind: "item", ID: i + 1, Field: "title", Before: value, After: value}
		}
		entry, ok := wslive.NewWhiteboardEffects(effects)
		if !ok || !entry.Truncated || len(entry.Text) > 64*1024 || !utf8.ValidString(entry.Text) || !json.Valid([]byte(entry.Text)) || utf8.RuneCountInString(entry.Caption) > 200 {
			t.Fatalf("invalid bounded carrier: ok %v, truncated %v, bytes %d", ok, entry.Truncated, len(entry.Text))
		}
		got, ok := wslive.DecodeWhiteboardEffects(entry)
		if !ok || len(got) < 1 || len(got) >= len(effects) {
			t.Fatalf("decode: %d effects, %v", len(got), ok)
		}
		for _, e := range got {
			if len(e.Before) > 4096 || len(e.After) > 4096 || !utf8.ValidString(e.Before+e.After) {
				t.Fatal("unbounded/invalid value")
			}
		}
		raw, _ := json.Marshal(entry)
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		if len(fields) != 4 {
			t.Fatalf("outer shape: %s", raw)
		}
		for _, k := range []string{"kind", "caption", "text", "truncated"} {
			if _, ok := fields[k]; !ok {
				t.Fatalf("missing outer field %s", k)
			}
		}
	}
}

func TestWhiteboardEffectsRetainsFirstWorstCaseEffect(t *testing.T) {
	// Even invalid caller data gets a bounded carrier/fallback, never a false
	// semantic-no-op signal. Exercise every string cap at once.
	huge := strings.Repeat("\x00", 70000)
	entry, ok := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{{Kind: huge, Field: huge, Topic: huge, EntryID: huge, PanelID: huge, Before: huge, After: huge}})
	var env struct {
		Version int                   `json:"v"`
		Effects []wswhiteboard.Effect `json:"effects"`
	}
	if !ok || !entry.Truncated || len(entry.Text) > 64*1024 || !utf8.ValidString(entry.Text) || json.Unmarshal([]byte(entry.Text), &env) != nil || env.Version != 1 || len(env.Effects) != 1 {
		t.Fatal("first effect not retained in complete bounded JSON")
	}
	if _, ok := wslive.DecodeWhiteboardEffects(entry); ok {
		t.Fatal("invalid variant interpreted")
	}
}

func TestWhiteboardEffectsTypedVariants(t *testing.T) {
	effects := []wswhiteboard.Effect{
		{Kind: "item", Topic: "legacy <topic>", ID: 1, Field: "title", Before: "old", After: "new"},
		{Kind: "topic", Topic: "topic", Field: "presence", After: "present"},
		{Kind: "panel", Topic: "topic", EntryID: "e1", PanelID: "p1", Field: "payload", After: "<script>alert(1)</script>"},
		{Kind: "panel", Topic: "topic", EntryID: "e1", PanelID: "p1", Field: "recap"},
		{Kind: "log", Topic: "topic", EntryID: "e2", Field: "text", After: "log"},
		{Kind: "setting", Field: "id-floor", Before: "1", After: "20"},
		{Kind: "setting", Field: "embed-dir", After: "/tmp"},
	}
	entry, _ := wslive.NewWhiteboardEffects(effects)
	got, ok := wslive.DecodeWhiteboardEffects(entry)
	if !ok || entry.Truncated || !reflect.DeepEqual(got, effects) {
		t.Fatalf("roundtrip: %+v, %v", got, ok)
	}
	for _, kind := range []string{"item", "topic", "panel", "log", "setting"} {
		for _, field := range wswhiteboard.EffectFields(kind) {
			e := effects[0]
			for _, candidate := range effects {
				if candidate.Kind == kind {
					e = candidate
					break
				}
			}
			e.Field = field
			if field == "presence" {
				e.Before, e.After = "", "present"
			}
			if kind == "topic" && field == "closed" {
				e.Before, e.After = "", "closed"
			}
			entry, _ := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{e})
			if _, ok := wslive.DecodeWhiteboardEffects(entry); !ok {
				t.Errorf("rejected %s/%s", kind, field)
			}
		}
	}
}

func TestWhiteboardEffectsRejectsAmbiguousUnsupportedOrUnbounded(t *testing.T) {
	valid := `{"v":1,"effects":[{"kind":"item","id":1,"field":"title","after":"new"}]}`
	bad := []string{
		`{`, strings.Replace(valid, `"v":1`, `"v":2`, 1), strings.Replace(valid, `"v":1`, `"v":1,"v":1`, 1),
		strings.Replace(valid, `"v":1`, `"V":1`, 1), strings.Replace(valid, `"title"`, `"payload"`, 1),
		strings.Replace(valid, `"item"`, `"future"`, 1), strings.Replace(valid, `"id":1`, `"id":-1`, 1),
		strings.Replace(valid, `"id":1`, `"id":1.5`, 1), strings.Replace(valid, `"id":1`, `"id":9007199254740992`, 1),
		strings.Replace(valid, `"id":1`, `"id":null`, 1), strings.Replace(valid, `"id":1`, `"id":1,"panelId":"p1"`, 1),
		strings.Replace(valid, `"id":1`, `"id":1,"unknown":"x"`, 1), valid + `{}`,
		`{"v":1,"effects":[]}`, `{"v":1,"effects":null}`, strings.Repeat(" ", 64*1024) + valid,
		strings.Replace(valid, "new", strings.Repeat("x", 4097), 1), strings.Replace(valid, "new", "\xff", 1),
		`{"v":1,"effects":[{"kind":"panel","topic":"a","entryId":"e01","panelId":"p1","field":"payload"}]}`,
		`{"v":1,"effects":[{"kind":"log","topic":"a","entryId":"e1","panelId":"p1","field":"text"}]}`,
		`{"v":1,"effects":[{"kind":"setting","topic":"a","field":"embed-dir"}]}`,
		`{"v":1,"effects":[{"kind":"item","id":1,"field":"presence","after":"invented"}]}`,
		`{"v":1,"effects":[{"kind":"topic","topic":"a","field":"closed","after":"present"}]}`,
		`{"v":1,"effects":[{"kind":"setting","field":"id-floor","before":"1","after":"NaN"}]}`,
		`{"v":1,"effects":[` + strings.TrimSuffix(strings.Repeat(`{"kind":"item","id":1,"field":"title"},`, 129), ",") + `]}`,
	}
	for _, text := range bad {
		entry := wslive.RingEntry{Kind: wslive.RingWhiteboardEffects, Text: text}
		if effects, ok := wslive.DecodeWhiteboardEffects(entry); ok || effects != nil {
			t.Errorf("accepted %.150q", text)
		}
		fallback := wslive.WhiteboardEffectsFallback(entry)
		if len(fallback) > 4096 || !utf8.ValidString(fallback) {
			t.Fatal("unsafe fallback bounds")
		}
	}
	for _, entry := range []wslive.RingEntry{{Kind: wslive.RingReply, Text: valid}, {Kind: wslive.RingWhiteboardEffects, Text: valid, Caption: strings.Repeat("雪", 201)}} {
		if _, ok := wslive.DecodeWhiteboardEffects(entry); ok {
			t.Fatal("accepted wrong kind or unbounded caption")
		}
	}
}

func TestWhiteboardEffectsPinnedOldDiskReader(t *testing.T) {
	// Pin actual pre-change source, not a newly invented tolerant struct.
	raw, err := os.ReadFile("testdata/pre_effects/ring.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(raw)); got != "428a0d1f2df6c475441495bd80ad4c2e9dc7ec80aba2027e985cc2d5419e8919" {
		t.Fatal("old reader fixture changed: " + got)
	}
	dir := t.TempDir()
	effect, _ := wslive.NewWhiteboardEffects([]wswhiteboard.Effect{{Kind: "item", ID: 1, Field: "title", After: "new"}})
	if err := wslive.AppendRing(dir, []wslive.RingEntry{{Kind: wslive.RingUser, Text: "legacy"}, effect, {Kind: wslive.RingReply, Text: "reply"}}); err != nil {
		t.Fatal(err)
	}
	// Under Phase 2's append-only .jsonl segment layout (T1), the pinned old
	// reader ignores .jsonl segments without error and preserves them while
	// writing legacy .json entries; the new reader merges both cleanly.
	old, err := oldring.ReadRing(dir)
	if err != nil || len(old) != 0 {
		t.Fatalf("old reader on segments: %+v %v", old, err)
	}
	if err := oldring.AppendRing(dir, []oldring.RingEntry{{Kind: oldring.RingUser, Text: "old-writer"}}); err != nil {
		t.Fatal(err)
	}
	merged, err := wslive.ReadRing(dir)
	if err != nil || len(merged) != 4 {
		t.Fatalf("new reader after old append: %+v %v", merged, err)
	}
	if merged[0].Text != "legacy" || merged[1].Kind != effect.Kind || merged[1].Text != effect.Text || merged[1].Caption != effect.Caption || merged[2].Text != "reply" || merged[3].Text != "old-writer" {
		t.Fatalf("merged ring lost history: %+v", merged)
	}
}
