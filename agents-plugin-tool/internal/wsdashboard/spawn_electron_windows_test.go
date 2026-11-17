//go:build windows

package wsdashboard

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

// Inject creation; no process or executable fixture is needed.
func TestElectronBreakawaySignalAndJobLifetimeMode(t *testing.T) {
	for _, tc := range []struct {
		name        string
		jobLifetime bool
		err         error
		wantSignal  bool
	}{
		{"success", false, nil, false},
		{"access-denied", false, fmt.Errorf("wrapped: %w", windows.ERROR_ACCESS_DENIED), true},
		{"other-error", false, windows.ERROR_FILE_NOT_FOUND, false},
		{"untyped-error", false, errors.New("Access is denied."), false},
		{"job-success", true, nil, false},
		{"job-error", true, windows.ERROR_ACCESS_DENIED, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var flags uint32
			var messages []string
			_, err := startElectronDetachedWith(tc.jobLifetime, func(value uint32) (*exec.Cmd, error) {
				flags = value
				return &exec.Cmd{}, tc.err
			}, func(message string) { messages = append(messages, message) })
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
			base := uint32(windows.CREATE_NO_WINDOW | syscall.CREATE_NEW_PROCESS_GROUP)
			wantFlags := base
			if !tc.jobLifetime {
				wantFlags |= windows.CREATE_BREAKAWAY_FROM_JOB
			}
			if flags != wantFlags {
				t.Fatalf("flags = %#x, want %#x", flags, wantFlags)
			}
			log := strings.Join(messages, "\n")
			if strings.Contains(log, "WS_DAEMON_BREAKAWAY_DENIED=5") != tc.wantSignal {
				t.Fatalf("wrong access-denied signal: %s", log)
			}
			if tc.jobLifetime && !strings.Contains(log, "mode=job-lifetime") || !tc.jobLifetime && !strings.Contains(log, "mode=breakaway") {
				t.Fatalf("spawn mode missing: %s", log)
			}
		})
	}
}

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
