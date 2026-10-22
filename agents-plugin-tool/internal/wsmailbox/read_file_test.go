package wsmailbox

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var errTransientRead = errors.New("transient hold")

// fakeReadClock drives readFileRetrying's budget without real time: sleep
// advances now by the requested back-off and records it.
type fakeReadClock struct {
	t      time.Time
	sleeps []time.Duration
}

func (c *fakeReadClock) now() time.Time { return c.t }

func (c *fakeReadClock) sleep(d time.Duration) {
	c.sleeps = append(c.sleeps, d)
	c.t = c.t.Add(d)
}

func isTestTransient(err error) bool { return errors.Is(err, errTransientRead) }

func TestReadFileRetryingRetriesTransientThenSucceeds(t *testing.T) {
	clock := &fakeReadClock{t: time.Unix(0, 0)}
	calls := 0
	read := func(string) ([]byte, error) {
		calls++
		if calls <= 3 {
			return nil, errTransientRead
		}
		return []byte("ok"), nil
	}
	raw, err := readFileRetrying("p", read, isTestTransient, clock.now, clock.sleep)
	if err != nil || string(raw) != "ok" {
		t.Fatalf("got (%q, %v), want (\"ok\", nil)", raw, err)
	}
	if calls != 4 {
		t.Fatalf("calls = %d, want 4", calls)
	}
	want := []time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 20 * time.Millisecond}
	if len(clock.sleeps) != len(want) {
		t.Fatalf("sleeps = %v, want %v", clock.sleeps, want)
	}
	for i := range want {
		if clock.sleeps[i] != want[i] {
			t.Fatalf("sleeps = %v, want %v", clock.sleeps, want)
		}
	}
}

func TestReadFileRetryingReturnsNonTransientAtOnce(t *testing.T) {
	clock := &fakeReadClock{t: time.Unix(0, 0)}
	other := errors.New("disk on fire")
	calls := 0
	read := func(string) ([]byte, error) {
		calls++
		return nil, other
	}
	_, err := readFileRetrying("p", read, isTestTransient, clock.now, clock.sleep)
	if !errors.Is(err, other) {
		t.Fatalf("err = %v, want %v", err, other)
	}
	if calls != 1 || len(clock.sleeps) != 0 {
		t.Fatalf("calls = %d sleeps = %v, want one call and no sleep", calls, clock.sleeps)
	}
}

func TestReadFileRetryingPassesMissingFileThrough(t *testing.T) {
	clock := &fakeReadClock{t: time.Unix(0, 0)}
	missing := &fs.PathError{Op: "open", Path: "p", Err: fs.ErrNotExist}
	calls := 0
	read := func(string) ([]byte, error) {
		calls++
		return nil, missing
	}
	_, err := readFileRetrying("p", read, isTestTransient, clock.now, clock.sleep)
	if !os.IsNotExist(err) {
		t.Fatalf("err = %v, want an os.IsNotExist error", err)
	}
	if err != error(missing) {
		t.Fatalf("err = %v, want the read error unwrapped", err)
	}
	if calls != 1 || len(clock.sleeps) != 0 {
		t.Fatalf("calls = %d sleeps = %v, want one call and no sleep", calls, clock.sleeps)
	}
}

func TestReadFileRetryingExhaustsBudgetWithLastError(t *testing.T) {
	start := time.Unix(0, 0)
	clock := &fakeReadClock{t: start}
	calls := 0
	var last error
	read := func(string) ([]byte, error) {
		calls++
		last = &fs.PathError{Op: "open", Path: "p", Err: errTransientRead}
		return nil, last
	}
	_, err := readFileRetrying("p", read, isTestTransient, clock.now, clock.sleep)
	if err != last {
		t.Fatalf("err = %v, want the last read error", err)
	}
	if !clock.t.After(start.Add(readRetryBudget)) {
		t.Fatalf("gave up at %v, before the %v budget elapsed", clock.t.Sub(start), readRetryBudget)
	}
	for _, d := range clock.sleeps {
		if d > 200*time.Millisecond {
			t.Fatalf("back-off %v exceeds the 200ms cap", d)
		}
	}
	if calls != len(clock.sleeps)+1 {
		t.Fatalf("calls = %d sleeps = %d, want one more read than sleeps", calls, len(clock.sleeps))
	}
}

// readFile must keep os.ReadFile's contract on every platform: content on
// success and an os.IsNotExist error for a missing file.
func TestReadFileMatchesOSReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mailbox.json")
	if _, err := readFile(path); !os.IsNotExist(err) {
		t.Fatalf("missing file err = %v, want os.IsNotExist", err)
	}
	if err := os.WriteFile(path, []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw, err := readFile(path)
	if err != nil || string(raw) != "body" {
		t.Fatalf("got (%q, %v), want (\"body\", nil)", raw, err)
	}
}
