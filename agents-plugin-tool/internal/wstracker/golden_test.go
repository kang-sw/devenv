package wstracker

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsledger"
)

// The golden files are written by testdata/gen-golden.js from the mockup
// reference engine (ai-docs/ref/tracker-mockup/engine.js) over its fixtures:
// per viewer clone, the events the clone holds with their protocol-owned
// verdicts, and the engine's view state for that clone.

type goldenEvent struct {
	Member  string          `json:"member"`
	Clone   string          `json:"clone"`
	Seq     int             `json:"seq"`
	Broken  string          `json:"broken"`
	TS      string          `json:"ts"`
	Type    string          `json:"type"`
	Origin  string          `json:"origin"`
	Payload json.RawMessage `json:"payload"`
	Seen    map[string]int  `json:"seen"`
	Verdict string          `json:"verdict"`
	Reason  string          `json:"reason"`
}

type goldenDoc struct {
	Store     string          `json:"store"`
	Viewer    string          `json:"viewer"`
	Now       string          `json:"now"`
	StaleDays int             `json:"staleDays"`
	Genesis   json.RawMessage `json:"genesis"`
	Events    []goldenEvent   `json:"events"`
	Expected  map[string]any  `json:"expected"`
}

var goldenVerdicts = map[string]wsledger.Verdict{
	"valid": wsledger.Valid, "waiting": wsledger.Waiting, "flagged": wsledger.Flagged, "ignored": wsledger.Ignored,
}

func loadGolden(t testing.TB, path string) (goldenDoc, *memSource) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc goldenDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var evs []wsledger.Event
	for _, g := range doc.Events {
		ts, err := time.Parse(time.RFC3339Nano, g.TS)
		if err != nil {
			t.Fatal(err)
		}
		v, ok := goldenVerdicts[g.Verdict]
		if !ok {
			t.Fatalf("unknown verdict %q", g.Verdict)
		}
		evs = append(evs, wsledger.Event{ID: wsledger.EventID{Member: g.Member, Clone: g.Clone, Seq: g.Seq, Broken: g.Broken},
			TS: ts, Origin: g.Origin, Type: g.Type, Payload: g.Payload, Seen: g.Seen, Verdict: v, Reason: g.Reason})
	}
	return doc, newMemSource(doc.Genesis, doc.Viewer, evs)
}

// goldenExcluded lists the expected fields the Go view does not carry, with
// the reason.
var goldenExcluded = map[string]string{
	"events[].local": "mock bookkeeping (_local): the caller fills unpushed state from the wsledger sync report",
	"events[].open":  "mock bookkeeping (_open): the open batch is the caller's",
	"events[].late":  "mock bookkeeping (_late)",
	"stats.commits":  "a wsledger event carries no commit identity; the caller fills it",
}

func TestGoldenParity(t *testing.T) {
	files, err := filepath.Glob("testdata/golden/*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden files (%v); run testdata/gen-golden.js", err)
	}
	for _, f := range files {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".json"), func(t *testing.T) {
			doc, src := loadGolden(t, f)
			now, err := time.Parse(time.RFC3339Nano, doc.Now)
			if err != nil {
				t.Fatal(err)
			}
			view := Derive(src, doc.Viewer, now, doc.StaleDays)
			raw, err := json.Marshal(view)
			if err != nil {
				t.Fatal(err)
			}
			var got map[string]any
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			want := doc.Expected
			for _, e := range want["events"].([]any) {
				ev := e.(map[string]any)
				delete(ev, "local")
				delete(ev, "open")
				delete(ev, "late")
			}
			delete(want["stats"].(map[string]any), "commits")
			delete(got["stats"].(map[string]any), "commits")
			var diffs []string
			diffJSON("", want, got, &diffs)
			if len(diffs) > 0 {
				if len(diffs) > 40 {
					diffs = append(diffs[:40], fmt.Sprintf("... and %d more", len(diffs)-40))
				}
				t.Errorf("view differs from the reference engine:\n%s", strings.Join(diffs, "\n"))
			}
		})
	}
}

// diffJSON lists the paths where two decoded JSON values differ.
func diffJSON(path string, want, got any, out *[]string) {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: want object, got %s", path, short(got)))
			return
		}
		for _, k := range sortedKeys(w) {
			gv, ok := g[k]
			if !ok {
				*out = append(*out, fmt.Sprintf("%s.%s: missing (want %s)", path, k, short(w[k])))
				continue
			}
			diffJSON(path+"."+k, w[k], gv, out)
		}
		for _, k := range sortedKeys(g) {
			if _, ok := w[k]; !ok {
				*out = append(*out, fmt.Sprintf("%s.%s: unexpected %s", path, k, short(g[k])))
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok {
			*out = append(*out, fmt.Sprintf("%s: want array %s, got %s", path, short(want), short(got)))
			return
		}
		if len(w) != len(g) {
			*out = append(*out, fmt.Sprintf("%s: want %d entries %s, got %d %s", path, len(w), short(want), len(g), short(got)))
			return
		}
		for i := range w {
			diffJSON(fmt.Sprintf("%s[%d]", path, i), w[i], g[i], out)
		}
	default:
		if !reflect.DeepEqual(want, got) {
			*out = append(*out, fmt.Sprintf("%s: want %s, got %s", path, short(want), short(got)))
		}
	}
}

func short(v any) string {
	b, _ := json.Marshal(v)
	s := string(b)
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}
