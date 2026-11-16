package mcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wsgit"
	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsmailbox"
)

type startupCountingRunner struct {
	mu    sync.Mutex
	calls [][]string
	run   func([]string) ([]byte, error)
}

func (r *startupCountingRunner) RunGit(_ context.Context, _ string, args ...string) ([]byte, error) {
	r.mu.Lock()
	r.calls = append(r.calls, append([]string(nil), args...))
	r.mu.Unlock()
	return r.run(args)
}

func (r *startupCountingRunner) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

func TestStartupGitCountRegistrationPaths(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, loginRoot := dashboardSessionServer(t, root, true)
	runner := &startupCountingRunner{run: func([]string) ([]byte, error) { return []byte(filepath.Join(root, ".git")), nil }}
	s.dashboard.commonRunner = runner
	s.dashboardTouch(key, loginRoot, false)
	if runner.count() != 1 {
		t.Fatalf("cold registration calls = %d", runner.count())
	}
	reg, ok := readRegistration(t, dir, key)
	if !ok || reg.GitCommonDir != filepath.Join(root, ".git") {
		t.Fatalf("registration = %+v", reg)
	}
	s.dashboardTouch(key, loginRoot, false)
	if runner.count() != 1 {
		t.Fatal("known row ran git")
	}

	// A daemon-owned row is detected before discovery.
	other := "daemon-owned-count-key"
	row := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(other), Project: loginRoot, Hosted: true}
	if err := wsdashboard.WriteRegistration(dir, row); err != nil {
		t.Fatal(err)
	}
	s.dashboardTouch(other, loginRoot, false)
	if runner.count() != 1 {
		t.Fatal("daemon row ran git")
	}
}

func TestStartupGitCountHosted(t *testing.T) {
	dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, loginRoot := dashboardSessionServer(t, root, true)
	// Resolve the hosted identity before dashboardTouch consults it.
	s.hostedOnce = sync.Once{}
	s.hostedOnce.Do(func() { s.hostedKey = key })
	runner := &startupCountingRunner{run: func([]string) ([]byte, error) { return nil, errors.New("unexpected git") }}
	s.dashboard.commonRunner = runner
	s.dashboardTouch(key, loginRoot, false)
	if runner.count() != 0 {
		t.Fatal("hosted session ran git")
	}
}

func TestStartupGitCountTakeoverDuringDiscovery(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, loginRoot := dashboardSessionServer(t, root, true)
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	s.dashboard.commonRunner = &startupCountingRunner{run: func([]string) ([]byte, error) {
		close(started)
		<-release
		return []byte(filepath.Join(root, ".git")), nil
	}}
	go func() { s.dashboardTouch(key, loginRoot, false); close(done) }()
	<-started
	row := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(key), Project: loginRoot, Hosted: true}
	if err := wsdashboard.WriteRegistration(dir, row); err != nil {
		close(release)
		<-done
		t.Fatal(err)
	}
	close(release)
	<-done
	reg, ok := readRegistration(t, dir, key)
	if !ok || !reg.Hosted {
		t.Fatalf("takeover overwritten: %+v", reg)
	}
}

// The session override is written directly to the authoritative key store,
// not through a keyed call which would itself wait for this discovery.
func TestStartupGitCountDisabledDuringDiscovery(t *testing.T) {
	for _, takeover := range []bool{false, true} {
		t.Run(fmt.Sprintf("hosted_takeover_%t", takeover), func(t *testing.T) {
			dir := dashboardSessionEnv(t)
			root := dashboardGitRoot(t)
			s, key, loginRoot := dashboardSessionServer(t, root, true)
			started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
			r := &startupCountingRunner{run: func([]string) ([]byte, error) {
				close(started)
				<-release
				return []byte(filepath.Join(root, ".git")), nil
			}}
			s.dashboard.commonRunner = r
			go func() { s.dashboardTouch(key, loginRoot, false); close(done) }()
			<-started
			if err := s.sessions.setOverride(key, DashboardEnabledKey, "false"); err != nil {
				close(release)
				<-done
				t.Fatal(err)
			}
			if takeover {
				row := wsdashboard.Registration{ReplyID: wsmailbox.ReplyID(key), Project: loginRoot, Hosted: true, HostPID: 4321}
				if err := wsdashboard.WriteRegistration(dir, row); err != nil {
					close(release)
					<-done
					t.Fatal(err)
				}
			}
			close(release)
			<-done
			reg, ok := readRegistration(t, dir, key)
			if takeover {
				if !ok || !reg.Hosted || reg.HostPID != 4321 || reg.GitCommonDir != "" {
					t.Fatalf("hosted ownership changed: %+v, present %t", reg, ok)
				}
			} else if ok {
				t.Fatalf("registered after disable: %+v", reg)
			}
			s.dashboard.mu.Lock()
			sess := s.dashboard.sessions[key]
			owned := sess.reg.ReplyID != ""
			enabled := sess.enabled
			s.dashboard.mu.Unlock()
			if owned || enabled || r.count() != 1 {
				t.Fatalf("owned=%t enabled=%t calls=%d", owned, enabled, r.count())
			}
		})
	}
}

func TestStartupGitCountConcurrentFullRegistrations(t *testing.T) {
	dir := dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, first, loginRoot := dashboardSessionServer(t, root, true)
	const n = 12
	keys := []string{first}
	for i := 1; i < n; i++ {
		key, gotRoot := parseLoginResponse(t, callLogin(t, s, 901000+i, root, nil))
		if gotRoot != loginRoot {
			t.Fatalf("root = %q, want %q", gotRoot, loginRoot)
		}
		keys = append(keys, key)
	}
	seen := make(map[string]bool)
	for _, key := range keys {
		if seen[key] {
			t.Fatalf("duplicate session key %q", key)
		}
		seen[key] = true
	}
	started, release := make(chan struct{}), make(chan struct{})
	var startOnce sync.Once
	r := &startupCountingRunner{run: func([]string) ([]byte, error) {
		startOnce.Do(func() { close(started) })
		<-release
		return []byte(filepath.Join(root, ".git")), nil
	}}
	s.dashboard.commonRunner = r
	var wg sync.WaitGroup
	wg.Add(n)
	for _, key := range keys {
		go func(key string) { defer wg.Done(); s.dashboardTouch(key, loginRoot, false) }(key)
	}
	<-started
	// While git is held, every full registration can acquire mu and reach
	// its shared discovery. This witnesses both no git under mu and actual
	// concurrent registrations rather than only direct cache calls.
	deadline := time.After(10 * time.Second)
	for {
		locked := make(chan int, 1)
		go func() { s.dashboard.mu.Lock(); locked <- len(s.dashboard.sessions); s.dashboard.mu.Unlock() }()
		select {
		case count := <-locked:
			if count == n {
				goto allWaiting
			}
		case <-deadline:
			close(release)
			wg.Wait()
			t.Fatal("registrations could not acquire dashboard mutex during git")
		}
		select {
		case <-deadline:
			close(release)
			wg.Wait()
			t.Fatal("registrations did not reach discovery")
		case <-time.After(time.Millisecond):
		}
	}
allWaiting:
	close(release)
	wg.Wait()
	if r.count() != 1 {
		t.Fatalf("shared root git calls = %d", r.count())
	}
	for _, key := range keys {
		reg, ok := readRegistration(t, dir, key)
		if !ok || reg.Project != loginRoot || reg.GitCommonDir != filepath.Join(root, ".git") {
			t.Fatalf("registration %q = %+v, present %t", key, reg, ok)
		}
	}
}

func TestStartupGitCountOff(t *testing.T) {
	dashboardSessionEnv(t)
	root := dashboardGitRoot(t)
	s, key, loginRoot := dashboardSessionServer(t, root, false)
	runner := &startupCountingRunner{run: func([]string) ([]byte, error) { return nil, errors.New("unexpected git") }}
	s.dashboard.commonRunner = runner
	s.dashboardTouch(key, loginRoot, false)
	if runner.count() != 0 {
		t.Fatal("off registration ran git")
	}
}

func TestStartupGitCountSharedRootAndRecovery(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	d := &dashboardLink{now: func() time.Time { return now }}
	started, release := make(chan struct{}), make(chan struct{})
	r := &startupCountingRunner{run: func([]string) ([]byte, error) {
		close(started)
		<-release
		return nil, errors.New("temporary failure")
	}}
	const n = 12
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() { defer wg.Done(); d.commonDir(r, "root") }()
	}
	<-started
	close(release)
	wg.Wait()
	if r.count() != 1 {
		t.Fatalf("shared failure calls = %d", r.count())
	}
	now = now.Add(dashboardCommonDirReuse)
	want := filepath.Join(t.TempDir(), "repo.git")
	r.run = func([]string) ([]byte, error) { return []byte(want), nil }
	if got := d.commonDir(r, "root"); got != want || r.count() != 2 {
		t.Fatalf("recovery = %q, calls %d", got, r.count())
	}
	now = now.Add(dashboardCommonDirReuse)
	moved := filepath.Join(t.TempDir(), "moved.git")
	r.run = func([]string) ([]byte, error) { return []byte(moved), nil }
	if got := d.commonDir(r, "root"); got != moved || r.count() != 3 {
		t.Fatalf("relocation = %q, calls %d", got, r.count())
	}
}

func TestSnapshotGitCountTwelveColdActiveSessions(t *testing.T) {
	roots := []string{filepath.Join(t.TempDir(), "main"), filepath.Join(t.TempDir(), "linked")}
	makeRunner := func() *startupCountingRunner {
		return &startupCountingRunner{run: func(args []string) ([]byte, error) {
			switch strings.Join(args, "\x00") {
			case "rev-parse\x00--short\x00HEAD":
				return []byte("abc123\n"), nil
			case "symbolic-ref\x00-q\x00HEAD":
				return []byte("refs/heads/main\n"), nil
			case "rev-parse\x00--abbrev-ref\x00--symbolic-full-name\x00@{upstream}":
				return []byte("origin/main\n"), nil
			case "rev-list\x00--left-right\x00--count\x00HEAD...@{upstream}":
				return []byte("2\t3\n"), nil
			case "--no-optional-locks\x00status\x00--porcelain\x00--untracked-files=normal":
				return []byte("MM file\nUU conflict\n?? new\n"), nil
			case "--no-optional-locks\x00status\x00--porcelain=v2\x00--branch\x00--ahead-behind\x00--untracked-files=normal":
				return []byte("# branch.oid abc123\n# branch.head main\n# branch.upstream origin/main\n# branch.ab +2 -3\n1 MM N... 100644 100644 100644 a b file\nu UU N... 100644 100644 100644 100644 a b c conflict\n? new\n"), nil
			default:
				t.Errorf("unrecognized git argv: %q", args)
				return nil, fmt.Errorf("unrecognized git argv: %q", args)
			}
		}}
	}
	legacy, current := makeRunner(), makeRunner()
	for i := 0; i < 12; i++ {
		root := roots[i%len(roots)]
		branch, ok := wsgit.BranchSync(context.Background(), legacy, root)
		if !ok {
			t.Fatal("legacy branch read failed")
		}
		counts, ok := wsgit.WorkingTreeCounts(context.Background(), legacy, root)
		if !ok {
			t.Fatal("legacy counts read failed")
		}
		want := wslive.Repo{Folder: filepath.Base(root), Branch: branch.Head, Upstream: branch.Upstream, Ahead: branch.Ahead, Behind: branch.Behind, Modified: counts.Modified, Staged: counts.Staged, Untracked: counts.Untracked}
		got := readSessionRepoWith(current, root)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("session %d: got %+v, legacy %+v", i, got, want)
		}
	}
	if legacy.count() != 12*5 || current.count() != 12 {
		t.Fatalf("active MCP snapshot git calls: legacy %d, current %d", legacy.count(), current.count())
	}
	t.Logf("12 cold active MCP snapshots: legacy %d git calls -> current %d (fixture, not Runner recovery or owner capture)", legacy.count(), current.count())
}

func TestSnapshotGitCountPorcelainParity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	r := &startupCountingRunner{run: func(args []string) ([]byte, error) {
		if !strings.Contains(strings.Join(args, " "), "--porcelain=v2") {
			t.Fatalf("unexpected args %v", args)
		}
		return []byte("# branch.oid abcdef\n# branch.head heads/main\n# branch.upstream origin/main\n# branch.ab +2 -3\n1 MM N... 100644 100644 100644 a b file\nu UU N... 100644 100644 100644 100644 a b c conflict\n? new\n"), nil
	}}
	got := readSessionRepoWith(r, root)
	if r.count() != 1 || got.Folder != "repo" || got.Branch != "heads/main" || got.Upstream != "origin/main" || got.Ahead != 2 || got.Behind != 3 || got.Modified != 2 || got.Staged != 1 || got.Untracked != 1 {
		t.Fatalf("repo = %+v; calls %d", got, r.count())
	}
}

func TestSnapshotGitCountInitialAndNoUpstream(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	for _, tc := range []struct{ status, branch string }{
		{"# branch.oid (initial)\n# branch.head main\n? new\n", ""},
		{"# branch.oid abcdef\n# branch.head main\n", "main"},
	} {
		r := &startupCountingRunner{run: func([]string) ([]byte, error) { return []byte(tc.status), nil }}
		got := readSessionRepoWith(r, root)
		if r.count() != 1 || got.Branch != tc.branch || got.Upstream != "" || got.Untracked != 0 {
			t.Fatalf("status %q: %+v, calls %d", tc.status, got, r.count())
		}
	}
}

func TestSnapshotGitCountDetachedAndFallback(t *testing.T) {
	root := filepath.Join(t.TempDir(), "repo")
	r := &startupCountingRunner{run: func(args []string) ([]byte, error) {
		if args[0] == "--no-optional-locks" {
			return []byte("# branch.oid abcdef\n# branch.head (detached)\n"), nil
		}
		return []byte("abc123\n"), nil
	}}
	got := readSessionRepoWith(r, root)
	if r.count() != 2 || !got.Detached || got.Branch != "abc123" {
		t.Fatalf("detached = %+v calls %d", got, r.count())
	}
	r = &startupCountingRunner{run: func(args []string) ([]byte, error) {
		switch args[0] {
		case "--no-optional-locks":
			return nil, errors.New("status failed")
		case "symbolic-ref":
			return []byte("refs/heads/main\n"), nil
		case "rev-list":
			return []byte("2\t1\n"), nil
		case "rev-parse":
			if args[1] == "--short" {
				return []byte("abc123\n"), nil
			}
			return []byte("origin/main\n"), nil
		}
		return nil, errors.New("unexpected")
	}}
	got = readSessionRepoWith(r, root)
	if r.count() != 5 || got.Branch != "main" || got.Upstream != "origin/main" || got.Ahead != 2 || got.Behind != 1 || got.Modified != 0 {
		t.Fatalf("fallback = %+v calls %d", got, r.count())
	}
}
