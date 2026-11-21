// Interrupt handling and delivery of held inbox entries.
package wsinbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// writeInterruptEntry commits Interrupt's entry (a test seam).
var writeInterruptEntry = writeEntry

// Interrupt claims every held message entry at dir, steered or not, removes
// them, and writes an interrupt entry whose SentAt is now. It returns the
// claimed records, in queue order (queueRecords), and the interrupt entry's
// name.
// A message another claim took first is skipped, and an unreadable one is
// left held, as Cancel leaves it. When the interrupt entry cannot be written,
// every claimed message is put back and the error returned.
func Interrupt(dir string, now time.Time) (held []Record, name string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", fmt.Errorf("create inbox dir: %w", err)
	}
	names, err := entryNames(dir)
	if err != nil {
		return nil, "", err
	}
	// An entry file is never rewritten, so reading it before the claim tells
	// whether the claimed file is a message.
	names = filterNames(dir, names, func(e Entry) bool { return !e.Interrupt })
	var claims []string
	putBack := func() {
		for i, c := range claims {
			_ = rename(c, filepath.Join(dir, held[i].Name))
		}
	}
	for _, n := range names {
		claimed, err := claim(dir, n, "interrupt")
		if errors.Is(err, ErrGone) {
			continue
		}
		if err != nil {
			putBack()
			return nil, "", err
		}
		entry, err := readEntry(claimed)
		if err != nil {
			_ = rename(claimed, filepath.Join(dir, n))
			continue
		}
		claims = append(claims, claimed)
		held = append(held, Record{Name: n, Entry: entry})
	}
	name, err = writeInterruptEntry(dir, Entry{SchemaVersion: SchemaVersion, Interrupt: true, SentAt: now.UnixMilli()})
	if err != nil {
		putBack()
		return nil, "", err
	}
	for _, c := range claims {
		_ = remove(c)
	}
	return queueRecords(held), name, nil
}

// Drain claims every held message at dir, moving each into the drained
// directory, and returns those this call claimed, in queue order
// (queueRecords): the steered ones first, then the rest. An entry another
// claim took first is skipped. Every interrupt entry it meets is claimed and
// deleted: never moved to the drained directory and never returned. The
// drained directory is then trimmed to the newest DrainedCap records, and
// the images only trimmed records listed are deleted.
func Drain(dir string) ([]Record, error) { return drain(dir, false) }

// DrainSteer is Drain restricted to steered entries: it claims only the held
// entries marked for steering and leaves every other one, interrupt entries
// included, held.
func DrainSteer(dir string) ([]Record, error) { return drain(dir, true) }

// DrainInterrupt claims only the interrupt entries at dir, oldest first,
// deletes them, keeping no drained record, and returns those this call
// claimed. Every message stays held.
func DrainInterrupt(dir string) ([]Record, error) {
	names, err := InterruptNames(dir)
	if err != nil || len(names) == 0 {
		return nil, err
	}
	return takeInterrupts(dir, names)
}

// takeInterrupts claims and deletes each named interrupt entry at dir and
// returns those it claimed. A claimed entry that does not read as an
// interrupt entry is put back.
func takeInterrupts(dir string, names []string) ([]Record, error) {
	var out []Record
	var firstErr error
	for _, name := range names {
		claimed, err := claim(dir, name, "drain")
		if err != nil {
			if !errors.Is(err, ErrGone) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		entry, err := readEntry(claimed)
		if err != nil || !entry.Interrupt {
			_ = rename(claimed, filepath.Join(dir, name))
			if err != nil && firstErr == nil {
				firstErr = err
			}
			continue
		}
		_ = remove(claimed)
		out = append(out, Record{Name: name, Entry: entry})
	}
	return out, firstErr
}

func drain(dir string, steerOnly bool) ([]Record, error) {
	names, err := entryNames(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if steerOnly {
		// An entry file is never rewritten, so reading it before the claim
		// tells whether the claimed file is steered.
		names = filterNames(dir, names, func(e Entry) bool { return e.Steer })
	} else {
		// Drop the interrupt entries; every other entry, an unreadable one
		// included, takes the drained path below. A failed drop leaves the
		// entry for the next full drain.
		interrupts := filterNames(dir, names, func(e Entry) bool { return e.Interrupt })
		if len(interrupts) > 0 {
			_, _ = takeInterrupts(dir, interrupts)
			names = without(names, interrupts)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	drained := DrainedDir(dir)
	if err := os.MkdirAll(drained, 0o755); err != nil {
		return nil, fmt.Errorf("create drained dir: %w", err)
	}
	var out []Record
	var firstErr error
	for _, name := range names {
		if err := rename(filepath.Join(dir, name), filepath.Join(drained, name)); err != nil {
			if !errors.Is(err, fs.ErrNotExist) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		entry, err := readEntry(filepath.Join(drained, name))
		if err != nil {
			// Put an unreadable entry back rather than lose it unreported.
			_ = rename(filepath.Join(drained, name), filepath.Join(dir, name))
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		out = append(out, Record{Name: name, Entry: entry})
	}
	trimDrained(dir)
	return queueRecords(out), firstErr
}

// queueRecords orders records, given in stamp order, as the inbox's two
// queues: the steered entries first, then the rest (follow-ups and command
// entries), each part keeping stamp order. A steer stamps the entry anew, so
// a promoted message lands at the end of the steer queue.
func queueRecords(records []Record) []Record {
	out := make([]Record, 0, len(records))
	for _, r := range records {
		if r.Entry.Steer {
			out = append(out, r)
		}
	}
	for _, r := range records {
		if !r.Entry.Steer {
			out = append(out, r)
		}
	}
	return out
}

// without returns names less every name in drop; both are sorted.
func without(names, drop []string) []string {
	out := names[:0:0]
	for _, name := range names {
		if len(drop) > 0 && drop[0] == name {
			drop = drop[1:]
			continue
		}
		out = append(out, name)
	}
	return out
}
