package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsgit"
)

type acquireRunnerFunc func(context.Context, string, ...string) ([]byte, error)

func (f acquireRunnerFunc) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	return f(ctx, root, args...)
}

func acquireTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func acquireWithServer(ctx context.Context, s *Server, runner wsgit.Runner, root, base, branch string, profile resolvedWorktreeProfile, pool, parent string) (worktreeAcquireResult, error) {
	return acquireWorktree(ctx, runner, s.environ(), root, base, branch, profile, pool, parent, func(path string) (string, error) {
		return s.sessions.mint(path, roleLead, parent)
	})
}

// Process gates live outside the checkout, so hygiene cannot remove them.
// They stop a real acquisition after its eligibility snapshot or inside mint,
// allowing another process to contend while the first still owns the lock.
type acquireProcessSpec struct {
	Root, Base, Branch, Profile, Pool, Parent, Cache, Gate, Stage, Result string
}

type acquireProcessResult struct {
	Acquired worktreeAcquireResult
	Error    string
}

func acquireFileWait(ctx context.Context, path string) error {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %s: %w", path, ctx.Err())
		case <-tick.C:
		}
	}
}

func TestWorktreeAcquireProcessHelper(t *testing.T) {
	path := os.Getenv("WS_TEST_ACQUIRE_SPEC")
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spec acquireProcessSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	s := NewServerWithOptions(ServerOptions{Root: spec.Root, Version: "test", Env: append(os.Environ(),
		"WS_CACHE_HOME="+spec.Cache, "WS_CONFIG_HOME="+filepath.Join(spec.Cache, "config"), "WS_MCP_NO_DASHBOARD=1")})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	signal := func(suffix string) {
		t.Helper()
		if err := os.WriteFile(spec.Gate+suffix, []byte("ready"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gate := func() {
		t.Helper()
		signal(".held")
		if err := acquireFileWait(ctx, spec.Gate+".resume"); err != nil {
			t.Fatal(err)
		}
	}
	var snapshot sync.Once
	runner := acquireRunnerFunc(func(callCtx context.Context, root string, args ...string) ([]byte, error) {
		out, err := s.gitRunner().RunGit(callCtx, root, args...)
		if err == nil && reflect.DeepEqual(args, []string{"rev-parse", "--path-format=absolute", "--git-common-dir"}) {
			signal(".attempt")
		}
		if err == nil && reflect.DeepEqual(args, []string{"worktree", "list", "--porcelain"}) {
			snapshot.Do(func() {
				signal(".snapshot")
				if spec.Stage == "snapshot" {
					gate()
				}
			})
		}
		return out, err
	})
	acq, acqErr := acquireWorktree(ctx, runner, s.environ(), spec.Root, spec.Base, spec.Branch,
		builtinProfile(t, spec.Profile), spec.Pool, spec.Parent, func(root string) (string, error) {
			if spec.Stage == "mint" {
				gate()
			}
			return s.sessions.mint(root, roleLead, spec.Parent)
		})
	res := acquireProcessResult{Acquired: acq}
	if acqErr != nil {
		res.Error = acqErr.Error()
	}
	data, err = json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec.Result, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func startAcquireProcess(t *testing.T, spec acquireProcessSpec) func() acquireProcessResult {
	t.Helper()
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	specPath := spec.Result + ".spec"
	if err := os.WriteFile(specPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWorktreeAcquireProcessHelper$")
	cmd.Env = append(os.Environ(), "WS_TEST_ACQUIRE_SPEC="+specPath)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		cancel() // only this test's own process may be cancelled
		<-done
	})
	return func() acquireProcessResult {
		t.Helper()
		<-done
		if waitErr != nil {
			t.Fatalf("acquire process: %v\n%s", waitErr, output.String())
		}
		data, err := os.ReadFile(spec.Result)
		if err != nil {
			t.Fatal(err)
		}
		var res acquireProcessResult
		if err := json.Unmarshal(data, &res); err != nil {
			t.Fatal(err)
		}
		if res.Error != "" {
			t.Fatalf("acquire process failed: %s\n%s", res.Error, output.String())
		}
		return res
	}
}

func waitAcquireGate(t *testing.T, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := acquireFileWait(ctx, path); err != nil {
		t.Fatal(err)
	}
}

func assertAcquiredLease(t *testing.T, s *Server, acq worktreeAcquireResult, branch, parent string) {
	t.Helper()
	if acq.Path == "" || acq.WorkerKey == "" || wtBranch(t, acq.Path) != branch {
		t.Fatalf("acquire = %+v, want branch %s and a bound worker key", acq, branch)
	}
	lease, err := readWorktreeLease(context.Background(), s.gitRunner(), acq.Path)
	if err != nil || lease == nil || lease.SchemaVersion != 1 || lease.WorkerKey != acq.WorkerKey || lease.ParentKey != parent {
		t.Fatalf("published lease = %+v, err %v, acquired %+v", lease, err, acq)
	}
	entry, ok := s.sessions.lookup(acq.WorkerKey)
	if !ok || entry.root != acq.Path || entry.parent != parent || entry.scope != roleLead {
		t.Fatalf("worker key binding = %+v, found %v, acquired %+v", entry, ok, acq)
	}
}

func TestWorktreeAcquireConcurrentProcesses(t *testing.T) {
	for _, profileName := range []string{worktreeProfileFull, worktreeProfileHousekeeping} {
		for _, idle := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/idle=%t", profileName, idle), func(t *testing.T) {
				root := profileWorktreeFixture(t)
				cache, pool, gates := t.TempDir(), acquireTempDir(t), t.TempDir()
				s := NewServerWithOptions(ServerOptions{Root: root, Version: "test", Env: append(os.Environ(), "WS_CACHE_HOME="+cache)})
				parent, err := s.sessions.mint(root, roleLead, "")
				if err != nil {
					t.Fatal(err)
				}
				profile := builtinProfile(t, profileName)
				var idlePath string
				if idle {
					seed, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, "develop", "impl/test/seed", profile, pool, parent)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := releaseWorktree(context.Background(), s.gitRunner(), s.environ(), seed.Path, pool, builtinProfileLookup(t)); err != nil {
						t.Fatal(err)
					}
					idlePath = seed.Path
				}
				// A caller rooted in a different linked checkout must use exactly
				// the same lock as the caller rooted in the primary checkout.
				linked := filepath.Join(t.TempDir(), "caller")
				runGit(t, root, "worktree", "add", "--detach", linked, "develop")
				first := acquireProcessSpec{Root: root, Base: "develop", Branch: "impl/test/first", Profile: profileName,
					Pool: pool, Parent: parent, Cache: cache, Gate: filepath.Join(gates, "first"), Stage: "snapshot", Result: filepath.Join(gates, "first.json")}
				// On the creation path pause later, in mint: protection must also
				// outlive provisioning and reach publication in a separate process.
				if !idle {
					first.Stage = "mint"
				}
				second := first
				second.Root, second.Branch, second.Stage = linked, "impl/test/second", ""
				second.Gate, second.Result = filepath.Join(gates, "second"), filepath.Join(gates, "second.json")
				finishFirst := startAcquireProcess(t, first)
				waitAcquireGate(t, first.Gate+".held")
				finishSecond := startAcquireProcess(t, second)
				waitAcquireGate(t, second.Gate+".attempt")
				// This short observation is not what makes the overlap happen:
				// first's explicit barrier still holds its snapshot/publication.
				ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
				err = acquireFileWait(ctx, second.Gate+".snapshot")
				cancel()
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("second process took a snapshot while the first was paused: %v", err)
				}
				if err := os.WriteFile(first.Gate+".resume", nil, 0o600); err != nil {
					t.Fatal(err)
				}
				a, b := finishFirst().Acquired, finishSecond().Acquired
				if a.Path == b.Path || a.WorkerKey == b.WorkerKey {
					t.Fatalf("concurrent acquires share ownership: %+v / %+v", a, b)
				}
				if idle && (!a.Reused || a.Path != idlePath || b.Reused) {
					t.Fatalf("one idle candidate must serve only first: %+v / %+v, idle %s", a, b, idlePath)
				}
				if !idle && (a.Reused || b.Reused) {
					t.Fatalf("both acquisitions must create when no idle entry exists: %+v / %+v", a, b)
				}
				assertAcquiredLease(t, s, a, first.Branch, parent)
				assertAcquiredLease(t, s, b, second.Branch, parent)
				for _, acq := range []worktreeAcquireResult{a, b} {
					if acq.Profile != profileName {
						t.Fatalf("wrong profile: %+v", acq)
					}
					assertExists(t, acq.Path, "ai-docs/tickets/idea/a.md", true)
					assertExists(t, acq.Path, "src/y.go", profileName == worktreeProfileFull)
				}
			})
		}
	}
}

func TestWorktreeAcquireLockTimeoutAndCancellation(t *testing.T) {
	s, root, base, parent := worktreeListServer(t)
	unlock, err := lockWorktreeAcquire(context.Background(), s.gitRunner(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	profile, pool := builtinProfile(t, worktreeProfileFull), filepath.Join(t.TempDir(), "not-created")
	// A waiter with the default budget times out even though the holder can
	// keep running hooks indefinitely; no eligibility snapshot may run.
	snapshots := 0
	runner := acquireRunnerFunc(func(ctx context.Context, root string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "worktree" {
			snapshots++
		}
		return s.gitRunner().RunGit(ctx, root, args...)
	})
	_, err = acquireWithServer(context.Background(), s, runner, root, base, "impl/test/timeout", profile, pool, parent)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "contention") {
		t.Fatalf("expected bounded contention error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := acquireWithServer(ctx, s, runner, root, base, "impl/test/cancelled", profile, pool, parent); err == nil {
		t.Fatal("cancelled waiter must fail")
	}
	if snapshots != 0 {
		t.Fatalf("wait failure reached worktree selection: %d snapshots", snapshots)
	}
	if _, err := os.Stat(pool); !os.IsNotExist(err) {
		t.Fatalf("wait failure created the pool: %v", err)
	}
	// A timed-out/cancelled waiter never removes the holder's lock.
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if release, err := lockWorktreeAcquire(ctx, s.gitRunner(), root); err == nil {
		release()
		t.Fatal("holder lost protection after a failed wait")
	}
	unlock()
	acq, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, "impl/test/later", profile, pool, parent)
	if err != nil {
		t.Fatalf("later acquisition cannot progress: %v", err)
	}
	assertAcquiredLease(t, s, acq, "impl/test/later", parent)
}

func TestWorktreeAcquireRollbackRetainsLock(t *testing.T) {
	for _, failure := range []string{"mint", "lease", "post_provision", "post_acquire", "ticket_board"} {
		t.Run(failure, func(t *testing.T) {
			s, root, base, parent := worktreeListServer(t)
			pool, profile := acquireTempDir(t), builtinProfile(t, worktreeProfileFull)
			if failure == "post_provision" || failure == "post_acquire" {
				profile = hookProfile(t, "build", nil, map[string][]map[string]any{
					failure: {hook(hookArgv(t, "exit", "3"))},
				})
			}
			if failure == "ticket_board" {
				// Add a tracked board so the deliberately narrow cone fails.
				writeFileForTest(t, root, "ai-docs/tickets/idea/a.md", "ticket")
				runGit(t, root, "add", ".")
				runGit(t, root, "commit", "-m", "board")
				base = strings.TrimSpace(string(runGitOutput(t, root, "rev-parse", "HEAD")))
				profile = customProfile(t, "narrow", `{"sparse":{"mode":"cone","paths":["src"]}}`)
			}
			seed, err := provisionWorktree(context.Background(), s.gitRunner(), s.environ(), root, base, "impl/test/seed", builtinProfile(t, worktreeProfileFull), pool)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := releaseWorktree(context.Background(), s.gitRunner(), s.environ(), seed.Path, pool, builtinProfileLookup(t)); err != nil {
				t.Fatal(err)
			}
			// Adopt the candidate into the failing profile, forcing hooks on
			// entry while keeping its path known before either acquisition.
			if err := writeWorktreeMarker(context.Background(), s.gitRunner(), seed.Path, worktreeMarker{Profile: profile.Name}); err != nil {
				t.Fatal(err)
			}
			var blocker string
			if failure == "lease" {
				admin, err := worktreeAdminDir(context.Background(), s.gitRunner(), seed.Path)
				if err != nil {
					t.Fatal(err)
				}
				blocker = filepath.Join(admin, worktreeLeaseFileName)
				if err := os.MkdirAll(filepath.Join(blocker, "block"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			held, resume, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			var once sync.Once
			resumeHolder := func() { once.Do(func() { close(resume) }) }
			t.Cleanup(resumeHolder)
			runner := acquireRunnerFunc(func(ctx context.Context, path string, args ...string) ([]byte, error) {
				if reflect.DeepEqual(args, noSubmoduleRecurse("checkout", "--detach")) {
					close(held)
					select {
					case <-resume:
					case <-time.After(10 * time.Second):
						return nil, errors.New("rollback test barrier timed out")
					}
				}
				return s.gitRunner().RunGit(ctx, path, args...)
			})
			go func() {
				_, err := acquireWorktree(context.Background(), runner, s.environ(), root, base, "impl/test/failed", profile, pool, parent, func(path string) (string, error) {
					if failure == "mint" {
						return "", errors.New("injected mint failure")
					}
					return s.sessions.mint(path, roleLead, parent)
				})
				done <- err
			}()
			select {
			case <-held:
			case err := <-done:
				t.Fatalf("acquire returned before rollback barrier: %v", err)
			case <-time.After(10 * time.Second):
				t.Fatal("rollback not reached")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			_, err = acquireWithServer(ctx, s, s.gitRunner(), root, base, "impl/test/waiting", profile, pool, parent)
			cancel()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("waiter must remain excluded through rollback: %v", err)
			}
			if b := wtBranch(t, seed.Path); b != "impl/test/failed" {
				t.Fatalf("waiter changed the holder's branch before rollback: %s", b)
			}
			resumeHolder()
			if err := <-done; err == nil || !strings.Contains(err.Error(), map[string]string{
				"mint": "mint worker key", "lease": "worktree lease", "post_provision": "quarantined", "post_acquire": "quarantined", "ticket_board": "hidden",
			}[failure]) {
				t.Fatalf("wrong failure: %v", err)
			}
			if b := wtBranch(t, seed.Path); b != "HEAD" {
				t.Fatalf("rollback did not detach: %s", b)
			}
			if blocker != "" {
				if err := os.RemoveAll(blocker); err != nil {
					t.Fatal(err)
				}
			}
			// Restore the profile definition; quarantine must still prevent
			// reuse. Other failures leave the same candidate safely reusable.
			healthy := profile
			healthy.Definition.Hooks, healthy.Definition.Sparse = nil, nil
			acq, err := acquireWithServer(context.Background(), s, s.gitRunner(), root, base, "impl/test/success", healthy, pool, parent)
			if err != nil {
				t.Fatalf("safe later acquisition failed: %v", err)
			}
			if quarantine := strings.HasPrefix(failure, "post_"); quarantine == (acq.Path == seed.Path) {
				t.Fatalf("quarantine/reuse mismatch after %s: %+v", failure, acq)
			}
			assertAcquiredLease(t, s, acq, "impl/test/success", parent)
			// A subsequent failing acquisition must not clean, detach or
			// overwrite this successful holder, even when it has ignored work.
			writeFileForTest(t, acq.Path, "worker-wip.txt", "owned work")
			writeFileForTest(t, acq.Path, "worker.log", "ignored owned work")
			_, err = acquireWorktree(context.Background(), s.gitRunner(), s.environ(), root, base, "impl/test/another-failure", healthy, pool, parent, func(string) (string, error) {
				return "", errors.New("injected mint failure")
			})
			if err == nil || !strings.Contains(err.Error(), "mint worker key") {
				t.Fatalf("expected a subsequent failed acquisition: %v", err)
			}
			assertExists(t, acq.Path, "worker-wip.txt", true)
			assertExists(t, acq.Path, "worker.log", true)
			assertAcquiredLease(t, s, acq, "impl/test/success", parent)
		})
	}
}
