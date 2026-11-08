package wstrackerhost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/kang-sw/devenv/internal/wstracker"
)

func TestSeenMarks(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	w.register(a, "team", "ada", &wstracker.Profile{Given: "Ada"})

	got, err := a.Seen("team")
	if err != nil {
		t.Fatal(err)
	}
	if got.Badge == nil || got.Read == nil || len(got.Badge)+len(got.Read) != 0 {
		t.Fatalf("a fresh store's marks = %+v, want empty non-nil lists", got)
	}

	if err := a.MarkSeen("team", Seen{Badge: []string{"b2", "b1", ""}, Read: []string{"r1"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.MarkSeen("team", Seen{Badge: []string{"b1", "b3"}, Read: []string{"", "r0", "r1"}}); err != nil {
		t.Fatal(err)
	}
	want := Seen{Badge: []string{"b1", "b2", "b3"}, Read: []string{"r0", "r1"}}
	if got, _ := a.Seen("team"); !reflect.DeepEqual(got, want) {
		t.Fatalf("marks = %+v, want %+v", got, want)
	}

	data, err := os.ReadFile(filepath.Join(a.Root(), "team", seenFile))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Version int      `json:"version"`
		Badge   []string `json:"badge"`
		Read    []string `json:"read"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || !reflect.DeepEqual(doc.Badge, want.Badge) || !reflect.DeepEqual(doc.Read, want.Read) {
		t.Fatalf("file = %s", data)
	}

	again, err := New(Options{Root: a.Root(), Clock: w.clock}) // a restart
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got, _ := again.Seen("team"); !reflect.DeepEqual(got, want) {
		t.Fatalf("after restart marks = %+v, want %+v", got, want)
	}

	// An empty mark list still writes empty arrays, never null.
	w.register(a, "solo", "ada", nil)
	if err := a.MarkSeen("solo", Seen{}); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(a.Root(), "solo", seenFile))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil || string(raw["badge"]) != "[]" || string(raw["read"]) != "[]" {
		t.Fatalf("empty marks file = %s (%v)", data, err)
	}
}

func TestSeenUnregistered(t *testing.T) {
	w := newWorld(t)
	a := w.machine("a")
	if _, err := a.Seen("nope"); err == nil {
		t.Fatal("Seen of an unregistered store must error")
	}
	if err := a.MarkSeen("nope", Seen{Badge: []string{"x"}}); err == nil {
		t.Fatal("MarkSeen of an unregistered store must error")
	}
}
