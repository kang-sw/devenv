package wsindex

import "testing"

// The CAS-loss classification is pinned on literal porcelain output so it
// holds whatever git version the suite runs under: the integration races
// only exercise the phrasing of the local git.
func TestPushStatusClassification(t *testing.T) {
	const src = "0123456789abcdef0123456789abcdef01234567"
	line := func(status string) string {
		return "To origin\n!\t" + src + ":" + RemoteRef + "\t" + status + "\nDone\n"
	}
	for _, tc := range []struct {
		name, status string
		lost         bool
	}{
		{"client lease check", "[rejected] (stale info)", true},
		{"non-fast-forward", "[rejected] (non-fast-forward)", true},
		{"fetch first", "[rejected] (fetch first)", true},
		{"receive-pack before 2.51", "[remote rejected] (failed to update ref)", true},
		{"lock detail", "[remote rejected] (cannot lock ref 'x': is at 1 but expected 2)", true},
		{"receive-pack batch commit failure", "[remote rejected] (failed to update refs)", true},
		{"receive-pack 2.51+ moved tip", "[remote rejected] (incorrect old value provided)", true},
		{"receive-pack 2.51+ deleted tip", "[remote rejected] (reference does not exist)", true},
		{"receive-pack 2.51+ create race", "[remote rejected] (reference already exists)", true},
		{"hook", "[remote rejected] (pre-receive hook declined)", false},
		{"hidden ref", "[remote rejected] (deny updating a hidden ref)", false},
		{"refname conflict", "[remote rejected] (refname conflict)", false},
	} {
		status, ok := porcelainStatus(line(tc.status))
		if !ok || status != tc.status {
			t.Fatalf("%s: porcelainStatus = %q, %v", tc.name, status, ok)
		}
		if got := isCASLoss(status); got != tc.lost {
			t.Errorf("%s: isCASLoss(%q) = %v, want %v", tc.name, status, got, tc.lost)
		}
	}
	if _, ok := porcelainStatus(line("[rejected] (stale info)") + "!\t" + src + ":refs/heads/other\t[rejected]\n"); !ok {
		t.Fatal("the index ref's status was not found beside another ref's")
	}
	if _, ok := porcelainStatus("!\t" + src + ":refs/heads/other\t[rejected] (stale info)\n"); ok {
		t.Fatal("another ref's rejection was read as the index ref's")
	}
}
