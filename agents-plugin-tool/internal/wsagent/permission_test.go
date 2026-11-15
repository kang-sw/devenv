package wsagent

import "testing"

func TestPermissionLeadMapping(t *testing.T) {
	for _, tc := range []struct {
		mode  string
		level PermissionLevel
	}{
		{"plan", PermissionReadOnly}, {"bypassPermissions", PermissionFull},
		{"", PermissionWorkdirWrite}, {"auto", PermissionWorkdirWrite},
		{"default", PermissionWorkdirWrite}, {"acceptEdits", PermissionWorkdirWrite}, {"dontAsk", PermissionWorkdirWrite},
	} {
		if got := legacyLeadPermission(Record{Lead: true, Backend: BackendClaude, PermissionMode: tc.mode}); got != tc.level {
			t.Fatalf("%s = %s, want %s", tc.mode, got, tc.level)
		}
	}
	for _, level := range LeadPermissionLevels() {
		if got := nativePermissionMode(BackendClaude, level); got == "" || legacyLeadPermission(Record{Lead: true, Backend: BackendClaude, PermissionMode: got}) != level {
			t.Fatalf("mapping %s = %s", level, got)
		}
		if got := nativePermissionMode(BackendPi, level); got != "" {
			t.Fatalf("invented Pi mode %s", got)
		}
		for _, active := range []string{"", "auto", "plan", "bypassPermissions"} {
			want := "advisory"
			if level == PermissionReadOnly && active == "plan" {
				want = "partial"
			}
			if got := permissionQuality(BackendClaude, level, active); got != want {
				t.Fatalf("quality %s/%s = %s", level, active, got)
			}
			if got := permissionQuality(BackendPi, level, active); got != "advisory" {
				t.Fatalf("Pi quality %s", got)
			}
		}
	}
	if got := legacyLeadPermission(Record{Lead: true, Backend: BackendPi}); got != PermissionWorkdirWrite {
		t.Fatal(got)
	}
	if got := legacyLeadPermission(Record{Lead: true, Backend: BackendClaude, PermissionMode: "unknown"}); got != "" {
		t.Fatalf("unknown mode fabricated %s", got)
	}
}

func TestPermissionLeadAuthority(t *testing.T) {
	for _, current := range LeadPermissionLevels() {
		for _, next := range LeadPermissionLevels() {
			for _, user := range []bool{false, true} {
				for _, ack := range []bool{false, true} {
					err := authorizePermission(current, next, user, ack)
					allowed := (next.rank() <= current.rank() || user) && (next != PermissionFull || (user && ack))
					if (err == nil) != allowed {
						t.Fatalf("%s -> %s user=%v ack=%v: %v", current, next, user, ack, err)
					}
				}
			}
		}
	}
	if authorizePermission(PermissionFull, "unknown", true, true) == nil {
		t.Fatal("invalid selection accepted")
	}
	if authorizePermission("", PermissionReadOnly, false, false) == nil {
		t.Fatal("unknown evidence granted agent authority")
	}
}
