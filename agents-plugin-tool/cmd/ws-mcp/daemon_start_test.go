package main

import "testing"

// TestDaemonStartLANFollowsTheApplyRule pins daemonStartLAN: a starter the
// dashboard applies to decides from its settings; any other starter leaves
// a running daemon's mode alone and otherwise takes the global file's
// value, never its own settings' false.
func TestDaemonStartLANFollowsTheApplyRule(t *testing.T) {
	running := func(lan, ok bool) func() (bool, bool) { return func() (bool, bool) { return lan, ok } }
	global := func(v bool) func() bool { return func() bool { return v } }
	for _, c := range []struct {
		name              string
		applies, settings bool
		running           func() (bool, bool)
		global            func() bool
		want              bool
	}{
		{"applies: settings true", true, true, running(false, true), global(false), true},
		{"applies: settings false", true, false, running(true, true), global(true), false},
		{"not applying: running LAN daemon stays", false, false, running(true, true), global(false), true},
		{"not applying: running loopback daemon stays", false, false, running(false, true), global(true), false},
		{"not applying, none running: global true", false, false, running(false, false), global(true), true},
		{"not applying, none running: global false", false, false, running(false, false), global(false), false},
	} {
		if got := daemonStartLAN(c.applies, c.settings, c.running, c.global); got != c.want {
			t.Errorf("%s: daemonStartLAN = %v, want %v", c.name, got, c.want)
		}
	}
}
