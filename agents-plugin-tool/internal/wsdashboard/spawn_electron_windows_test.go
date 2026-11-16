//go:build windows

package wsdashboard

import (
	"errors"
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

// Inject creation; no process or executable fixture is needed.
func TestWindowsStrictBreakawayDoesNotFallback(t *testing.T) {
	refused := errors.New("job forbids breakaway")
	for _, tc := range []struct {
		strict    bool
		wantCalls int
	}{{true, 1}, {false, 2}} {
		var flags []uint32
		_, err := startWindowsDetachedWith(tc.strict, func(flag uint32) (*exec.Cmd, error) {
			flags = append(flags, flag)
			if len(flags) == 1 {
				return nil, refused
			}
			return &exec.Cmd{}, nil
		})
		if len(flags) != tc.wantCalls {
			t.Fatalf("strict=%v: flags=%v", tc.strict, flags)
		}
		if flags[0]&windows.CREATE_BREAKAWAY_FROM_JOB == 0 {
			t.Fatal("first attempt must request breakaway")
		}
		if tc.strict {
			if !errors.Is(err, refused) {
				t.Fatalf("strict refusal = %v", err)
			}
		} else if err != nil || flags[1]&windows.CREATE_BREAKAWAY_FROM_JOB != 0 {
			t.Fatalf("default fallback = %v, flags %v", err, flags)
		}
	}
}
