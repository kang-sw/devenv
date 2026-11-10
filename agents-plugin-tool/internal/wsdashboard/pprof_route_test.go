package wsdashboard

import (
	"bytes"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

// TestPprofOptInRecordsContention: a daemon started without the pprof
// routes leaves the contention profiles off; one started with them turns
// on the mutex profile's sampling and the block profile, which then holds
// a wait longer than its rate.
func TestPprofOptInRecordsContention(t *testing.T) {
	runtime.SetMutexProfileFraction(0)
	runtime.SetBlockProfileRate(0)
	t.Cleanup(func() {
		runtime.SetMutexProfileFraction(0)
		runtime.SetBlockProfileRate(0)
	})

	startPageFixture(t)
	if got := runtime.SetMutexProfileFraction(-1); got != 0 {
		t.Fatalf("mutex profile fraction without the opt-in = %d, want 0", got)
	}

	startPageFixture(t, func(o *DaemonOptions) { o.Pprof = true })
	if got := runtime.SetMutexProfileFraction(-1); got != pprofMutexFraction {
		t.Fatalf("mutex profile fraction with the opt-in = %d, want %d", got, pprofMutexFraction)
	}

	// A receive that waits well past the block rate is always recorded.
	ch := make(chan struct{})
	go func() {
		time.Sleep(5 * time.Duration(pprofBlockRate))
		close(ch)
	}()
	blockedForProfile(ch)
	var buf bytes.Buffer
	if err := pprof.Lookup("block").WriteTo(&buf, 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "blockedForProfile") {
		t.Fatalf("block profile with the opt-in holds no wait of blockedForProfile:\n%.2000s", buf.String())
	}
}

//go:noinline
func blockedForProfile(ch chan struct{}) { <-ch }
