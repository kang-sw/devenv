package wsagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// No fixture launches a process, including the Windows shim cases on Linux.
type piProbeFixture struct {
	t                            *testing.T
	root, command, node          string
	now                          time.Time
	models, version              string
	modelsErr, versionErr        error
	modelCalls, versionCalls     int
	modelCommand, versionCommand string
	modelArgs, versionArgs       []string
	duringModels, duringVersion  func()
}

func newPiProbeFixture(t *testing.T) *piProbeFixture {
	t.Helper()
	root := t.TempDir()
	for key, dir := range map[string]string{
		"HOME": "home", "USERPROFILE": "home", "CLAUDE_CONFIG_DIR": "claude", "CODEX_HOME": "codex",
		"WS_CACHE_HOME": "cache", "WS_CONFIG_HOME": "config", "TMPDIR": "tmp", "PI_CODING_AGENT_DIR": "agent",
	} {
		path := filepath.Join(root, dir)
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv(key, path)
	}
	f := &piProbeFixture{t: t, root: root, command: filepath.Join(root, "bin", "pi"), node: filepath.Join(root, "bin", "node"),
		now: time.Date(2026, 11, 16, 1, 0, 0, 123, time.UTC), models: piListing, version: "pi 1.0.4\n"}
	f.write(f.command, "mock command")
	f.write(f.node, "mock node")
	f.write(filepath.Join(root, "package.json"), `{"name":"fixture"}`)
	setHostWindows(t, false)
	setLookPath(t, func(name string) (string, error) {
		switch name {
		case "pi":
			return f.command, nil
		case "node":
			return f.node, nil
		}
		if filepath.IsAbs(name) && fileExists(name) {
			return name, nil
		}
		return "", exec.ErrNotFound
	})
	oldNow, oldModels, oldVersion := piProbeNow, runListing, runPiVersion
	piProbeNow = func() time.Time { return f.now }
	runListing = func(ctx context.Context, command string, args, env []string) ([]byte, error) {
		f.modelCalls++
		f.modelCommand, f.modelArgs = command, append([]string(nil), args...)
		if !reflect.DeepEqual(args, []string{"--offline", "--no-approve", "--list-models"}) {
			t.Fatalf("listing argv changed: %v", args)
		}
		if !containsEnv(env, "PI_SKIP_VERSION_CHECK=1") {
			t.Fatal("listing lost piEnv")
		}
		if f.duringModels != nil {
			f.duringModels()
		}
		return []byte(f.models), f.modelsErr
	}
	runPiVersion = func(ctx context.Context, command string, args, env []string) ([]byte, error) {
		f.versionCalls++
		f.versionCommand, f.versionArgs = command, append([]string(nil), args...)
		if !containsEnv(env, "PI_SKIP_VERSION_CHECK=1") {
			t.Fatal("version lost piEnv")
		}
		if f.duringVersion != nil {
			f.duringVersion()
		}
		return []byte(f.version), f.versionErr
	}
	t.Cleanup(func() { piProbeNow, runListing, runPiVersion = oldNow, oldModels, oldVersion })
	return f
}

func containsEnv(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}

func (f *piProbeFixture) write(path, text string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *piProbeFixture) probe() {
	f.t.Helper()
	if got, err := PiVersion(context.Background()); err != nil || got != "1.0.4" {
		f.t.Fatalf("version = %q, %v", got, err)
	}
	models, efforts, err := (PiBackend{}).Choices(context.Background())
	if err != nil || len(models) != 3 || !reflect.DeepEqual(efforts, piEfforts) {
		f.t.Fatalf("choices = %v, %v, %v", models, efforts, err)
	}
}

func (f *piProbeFixture) cache(kind string) piProbeCache {
	f.t.Helper()
	command, pre := f.command, []string(nil)
	if kind == "version" {
		var err error
		command, pre, err = harnessCommand("", "pi")
		if err != nil {
			f.t.Fatal(err)
		}
	}
	c := newPiProbeCache(kind, command, pre, piEnv(environ()))
	if c.path == "" {
		f.t.Fatalf("no %s cache identity", kind)
	}
	return c
}

func (f *piProbeFixture) support() *Support {
	return NewSupport(SupportOptions{
		Runner: &Runner{opts: Options{Log: io.Discard, Backends: map[string]BackendRegistration{BackendPi: {Backend: PiBackend{}}}}},
		Dir:    filepath.Join(os.Getenv("WS_CACHE_HOME"), "dashboard"), Now: func() time.Time { return f.now },
	})
}

func (f *piProbeFixture) choices(s *Support) SupportHarness {
	f.t.Helper()
	hs := []SupportHarness{{Harness: BackendPi, Available: true, Models: []ModelChoice{}, Efforts: []string{}}}
	s.listChoices(context.Background(), hs)
	return hs[0]
}

func TestPiProbeCacheRestartReuseAndPrivateParsedValues(t *testing.T) {
	f := newPiProbeFixture(t)
	secret := "credential-DO-NOT-PERSIST"
	f.write(filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "auth.json"), secret)
	f.write(filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "models.json"), secret)
	t.Setenv("ANTHROPIC_API_KEY", secret)
	t.Setenv("WS_PI_ROLE", "private-role-marker")
	f.models = "private-raw-banner\n" + piListing
	f.version = "private-version-banner 1.0.4\n"
	f.probe()
	first := f.support()
	if h := f.choices(first); h.Error != "" || len(h.Models) != 3 {
		t.Fatalf("disk choices = %+v", h)
	}
	second := f.support() // no shared memory cache
	if h := f.choices(second); h.Error != "" || len(h.Models) != 3 {
		t.Fatalf("restart choices = %+v", h)
	}
	f.probe() // fresh PiBackend value and a fresh PiVersion call
	if f.modelCalls != 1 || f.versionCalls != 1 {
		t.Fatalf("restart ran probes: %d models, %d version", f.modelCalls, f.versionCalls)
	}
	if f.modelCommand != f.command || f.versionCommand != f.command || !reflect.DeepEqual(f.versionArgs, []string{"--version"}) {
		t.Fatal("probe execution selection changed")
	}
	for _, kind := range []string{"version", "models"} {
		c := f.cache(kind)
		raw, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, banned := range []string{secret, "private-raw-banner", "private-version-banner", "private-role-marker", "permissions", "profiles", "error"} {
			if bytes.Contains(raw, []byte(banned)) {
				t.Fatalf("cache contains %q: %s", banned, raw)
			}
		}
		var shape map[string]json.RawMessage
		if json.Unmarshal(raw, &shape) != nil || len(shape) != 5 {
			t.Fatalf("unexpected record: %s", raw)
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(c.path)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("cache mode: %v, %v", info, err)
			}
			info, err = os.Stat(filepath.Dir(c.path))
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Fatalf("cache dir mode: %v, %v", info, err)
			}
		}
	}
	f.now = f.now.Add(365 * 24 * time.Hour)
	if v, err := PiVersion(context.Background()); err != nil || v != "1.0.4" || f.versionCalls != 1 {
		t.Fatalf("version success expired: %q %v (%d)", v, err, f.versionCalls)
	}
}

func TestPiProbeCacheModelExpiryUsesObservation(t *testing.T) {
	f := newPiProbeFixture(t)
	f.duringModels = func() { f.now = f.now.Add(choicesTimeout) }
	if h := f.choices(f.support()); h.Error != "" {
		t.Fatal(h.Error)
	}
	observed := f.now // completion, not probe start
	f.duringModels = nil
	f.now = observed.Add(4 * time.Minute)
	restarted := f.support()
	f.choices(restarted)
	if at := restarted.cache[BackendPi].at; !at.Equal(observed) {
		t.Fatalf("restart renewed observation: %v, want %v", at, observed)
	}
	f.now = observed.Add(choicesTTL - time.Nanosecond)
	f.choices(restarted)
	if f.modelCalls != 1 {
		t.Fatal("fresh listing probed")
	}
	f.now = observed.Add(choicesTTL)
	f.choices(restarted)
	if f.modelCalls != 2 {
		t.Fatal("restart extended model TTL")
	}
}

func TestPiProbeCacheCommandInvalidation(t *testing.T) {
	for _, change := range []string{"size", "nanosecond mtime", "command", "PATH", "symlink target", "node PATH"} {
		t.Run(change, func(t *testing.T) {
			f := newPiProbeFixture(t)
			if change == "node PATH" {
				f.write(f.command, "#!/usr/bin/env node\n// mock CLI")
			}
			if change == "symlink target" {
				target := filepath.Join(f.root, "one", "cli.js")
				f.write(target, "mock CLI")
				if err := os.Remove(f.command); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, f.command); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			if change == "PATH" {
				if err := os.Chmod(f.command, 0o700); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", filepath.Dir(f.command))
				setLookPath(t, exec.LookPath) // lookup only; execution is still mocked
			}
			f.probe()
			old := f.cache("models").path
			switch change {
			case "size":
				info, _ := os.Stat(f.command)
				f.write(f.command, "larger mock command")
				if err := os.Chtimes(f.command, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			case "nanosecond mtime":
				info, _ := os.Stat(f.command)
				at := info.ModTime().Add(time.Nanosecond)
				if err := os.Chtimes(f.command, at, at); err != nil {
					t.Fatal(err)
				}
			case "command", "PATH":
				f.command = filepath.Join(f.root, "other", "pi")
				f.write(f.command, "mock command")
				if change == "PATH" {
					if err := os.Chmod(f.command, 0o700); err != nil {
						t.Fatal(err)
					}
					t.Setenv("PATH", filepath.Dir(f.command))
				}
			case "symlink target":
				target := filepath.Join(f.root, "two", "cli.js")
				f.write(target, "mock CLI")
				if err := os.Remove(f.command); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, f.command); err != nil {
					t.Fatal(err)
				}
			case "node PATH":
				f.node = filepath.Join(f.root, "other", "node")
				f.write(f.node, "other node")
			}
			f.probe()
			if f.cache("models").path == old || f.modelCalls != 2 || f.versionCalls != 2 {
				t.Fatalf("identity reused after %s: models %d, version %d", change, f.modelCalls, f.versionCalls)
			}
		})
	}
}

func TestPiProbeCacheShimIdentityDoesNotRewireExecution(t *testing.T) {
	for _, change := range []string{"shim", "node", "CLI", "package", "nearer package appears", "beside node appears"} {
		t.Run(change, func(t *testing.T) {
			f := newPiProbeFixture(t)
			setHostWindows(t, true)
			f.command = filepath.Join(f.root, "npm", "pi.cmd")
			f.write(f.command, readPiShim(t))
			cli := filepath.Join(filepath.Dir(f.command), piShimScript)
			pkg := filepath.Join(filepath.Dir(f.command), "node_modules", "@earendil-works", "pi-coding-agent", "package.json")
			f.write(cli, "mock CLI")
			f.write(pkg, `{"version":"1.0.4"}`)
			f.probe()
			if f.modelCommand != f.command || f.versionCommand != f.node || !reflect.DeepEqual(f.versionArgs, []string{cli, "--version"}) {
				t.Fatalf("probes rewired: models %s, version %s %v", f.modelCommand, f.versionCommand, f.versionArgs)
			}
			old := f.cache("version").path
			switch change {
			case "shim":
				f.write(f.command, readPiShim(t)+"\r\nrem changed\r\n")
			case "node":
				f.write(f.node, "updated node")
			case "CLI":
				f.write(cli, "updated CLI")
			case "package":
				f.write(pkg, `{"version":"1.0.5","updated":true}`)
			case "nearer package appears":
				f.write(filepath.Join(filepath.Dir(cli), "package.json"), `{"version":"nested"}`)
			case "beside node appears":
				f.write(filepath.Join(filepath.Dir(f.command), "node.exe"), "new beside node")
			}
			f.probe()
			if f.cache("version").path == old || f.modelCalls != 2 || f.versionCalls != 2 {
				t.Fatalf("shim identity reused after %s", change)
			}
			if f.modelCommand != f.command {
				t.Fatal("listing started using harnessCommand")
			}
		})
	}
}

func TestPiProbeCacheConfigIdentityAndAbsences(t *testing.T) {
	for _, change := range []string{"agent dir", "HOME default", "tilde", "relative", "auth appears", "models appears", "auth disappears", "models disappears", "auth mtime", "models size", "package appears", "package disappears"} {
		t.Run(change, func(t *testing.T) {
			f := newPiProbeFixture(t)
			agent := os.Getenv("PI_CODING_AGENT_DIR")
			auth, models := filepath.Join(agent, "auth.json"), filepath.Join(agent, "models.json")
			pkg := filepath.Join(f.root, "package.json")
			switch change {
			case "HOME default":
				t.Setenv("PI_CODING_AGENT_DIR", "")
			case "auth disappears", "auth mtime":
				f.write(auth, "secret-a")
			case "models disappears", "models size":
				f.write(models, "secret-m")
			case "package appears":
				if err := os.Remove(pkg); err != nil {
					t.Fatal(err)
				}
			}
			f.probe()
			oldModels, oldVersion := f.cache("models").path, f.cache("version").path
			switch change {
			case "agent dir":
				t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(f.root, "other-agent"))
			case "HOME default":
				t.Setenv("HOME", filepath.Join(f.root, "other-home"))
				t.Setenv("USERPROFILE", filepath.Join(f.root, "other-home"))
			case "tilde":
				t.Setenv("PI_CODING_AGENT_DIR", "~/agent")
			case "relative":
				cwd, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				rel, err := filepath.Rel(cwd, agent)
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv("PI_CODING_AGENT_DIR", rel)
			case "auth appears":
				f.write(auth, "credential-a")
			case "models appears":
				f.write(models, "credential-m")
			case "auth disappears":
				if err := os.Remove(auth); err != nil {
					t.Fatal(err)
				}
			case "models disappears":
				if err := os.Remove(models); err != nil {
					t.Fatal(err)
				}
			case "auth mtime":
				info, _ := os.Stat(auth)
				at := info.ModTime().Add(time.Nanosecond)
				if err := os.Chtimes(auth, at, at); err != nil {
					t.Fatal(err)
				}
			case "models size":
				f.write(models, "changed credential-m")
			case "package appears":
				f.write(pkg, `{"name":"new"}`)
			case "package disappears":
				if err := os.Remove(pkg); err != nil {
					t.Fatal(err)
				}
			}
			f.probe()
			wantModels, wantVersion := 2, 1
			if change == "relative" { // the same effective directory is the same identity
				wantModels = 1
				if f.cache("models").path != oldModels {
					t.Fatal("relative spelling changed identity")
				}
			} else if f.cache("models").path == oldModels {
				t.Fatalf("models identity unchanged after %s", change)
			}
			if strings.HasPrefix(change, "package") {
				wantVersion = 2
			} else if f.cache("version").path != oldVersion {
				t.Fatal("model config changed version identity")
			}
			if f.modelCalls != wantModels || f.versionCalls != wantVersion {
				t.Fatalf("%s: models %d, version %d", change, f.modelCalls, f.versionCalls)
			}
		})
	}
}

func TestPiProbeCacheConfigNormalizationIsConservative(t *testing.T) {
	f := newPiProbeFixture(t)
	f.probe()
	if runtime.GOOS != "windows" {
		t.Setenv("pi_coding_agent_dir", filepath.Join(f.root, "ignored"))
		f.probe()
		if f.modelCalls != 1 {
			t.Fatal("POSIX environment lookup ignored case")
		}
		f.write(filepath.Join(os.Getenv("PI_CODING_AGENT_DIR"), "auth.json"), "secret")
		f.probe()
		if f.modelCalls != 2 {
			t.Fatal("actual agent directory was not stamped")
		}
		t.Setenv("PI_CODING_AGENT_DIR", `~\agent`)
		want, err := filepath.Abs(`~\agent`)
		if err != nil {
			t.Fatal(err)
		}
		if got := f.cache("models").identity.AgentDir; got != want {
			t.Fatalf("POSIX backslash was expanded: %q, want %q", got, want)
		}
	}
	t.Setenv("PI_CODING_AGENT_DIR", "file:///unresolved/agent")
	if c := newPiProbeCache("models", f.command, nil, piEnv(environ())); c.path != "" {
		t.Fatal("file URL was mistaken for a native config directory")
	}
	before := f.modelCalls
	f.probe()
	f.probe()
	if f.modelCalls != before+2 {
		t.Fatal("uncertain config identity did not fall back to probing")
	}
}

func TestPiProbeCacheInvalidRecordsAndFuturePreservation(t *testing.T) {
	for _, kind := range []string{"models", "version"} {
		for _, damage := range []string{"corrupt", "future schema", "kind", "identity", "zero time", "future time", "value", "duplicate models"} {
			t.Run(kind+"/"+damage, func(t *testing.T) {
				f := newPiProbeFixture(t)
				f.probe()
				c := f.cache(kind)
				raw, err := os.ReadFile(c.path)
				if err != nil {
					t.Fatal(err)
				}
				var r piProbeRecord
				if err := json.Unmarshal(raw, &r); err != nil {
					t.Fatal(err)
				}
				switch damage {
				case "corrupt":
				case "future schema":
					r.SchemaVersion++
				case "kind":
					r.Kind = "other"
				case "identity":
					r.Identity.Files[0].MtimeNS++
				case "zero time":
					r.ObservedAt = time.Time{}
				case "future time":
					r.ObservedAt = f.now.Add(time.Second)
				case "value":
					if kind == "models" {
						r.Value = json.RawMessage(`[{"id":"bad id"}]`)
					} else {
						r.Value = json.RawMessage(`"raw prefix 1.0.4 raw suffix"`)
					}
				case "duplicate models":
					if kind == "models" {
						r.Value = json.RawMessage(`[{"id":"a/b"},{"id":"a/b"}]`)
					} else {
						r.Value = json.RawMessage(`42`)
					}
				}
				if damage == "corrupt" {
					raw = []byte("{corrupt")
				} else {
					raw, err = json.Marshal(r)
					if err != nil {
						t.Fatal(err)
					}
				}
				f.write(c.path, string(raw))
				f.probe()
				wantModels, wantVersion := 1, 1
				if kind == "models" {
					wantModels++
				} else {
					wantVersion++
				}
				if f.modelCalls != wantModels || f.versionCalls != wantVersion {
					t.Fatalf("invalid %s reused: %d %d", damage, f.modelCalls, f.versionCalls)
				}
				if damage == "future schema" {
					after, err := os.ReadFile(c.path)
					if err != nil || !bytes.Equal(after, raw) {
						t.Fatalf("future schema rewritten: %s %v", after, err)
					}
				}
			})
		}
	}
}

func TestPiProbeCacheFailuresAreNotPersistent(t *testing.T) {
	for _, failure := range []string{"execution", "parse"} {
		t.Run(failure, func(t *testing.T) {
			f := newPiProbeFixture(t)
			if failure == "execution" {
				f.modelsErr, f.versionErr = errors.New("private failure"), errors.New("private failure")
			} else {
				f.models, f.version = "private unrecognized output", "private no version"
			}
			for i := 0; i < 2; i++ {
				if _, err := PiVersion(context.Background()); err == nil {
					t.Fatal("version failure hidden")
				}
				if _, _, err := (PiBackend{}).Choices(context.Background()); err == nil {
					t.Fatal("listing failure hidden")
				}
			}
			if f.modelCalls != 2 || f.versionCalls != 2 {
				t.Fatal("failure persisted")
			}
			for _, kind := range []string{"models", "version"} {
				if _, err := os.Stat(f.cache(kind).path); !os.IsNotExist(err) {
					t.Fatalf("failure cache exists: %v", err)
				}
			}
		})
	}
}

func TestPiProbeCacheFailureTTLAndOldSuccessRetention(t *testing.T) {
	f := newPiProbeFixture(t)
	f.probe()
	c := f.cache("models")
	before, _ := os.ReadFile(c.path)
	f.now = f.now.Add(choicesTTL)
	f.modelsErr = errors.New("listing failed")
	f.duringModels = func() { f.now = f.now.Add(choicesTimeout) }
	s := f.support()
	if h := f.choices(s); h.Error == "" {
		t.Fatal("stale success hid failure")
	}
	ended := f.now
	f.duringModels = nil
	f.now = ended.Add(choicesFailTTL - time.Nanosecond)
	f.choices(s)
	if f.modelCalls != 2 {
		t.Fatal("memory failure TTL changed")
	}
	f.now = ended.Add(choicesFailTTL)
	f.choices(s)
	if f.modelCalls != 3 {
		t.Fatal("memory failure did not expire")
	}
	f.choices(f.support())
	if f.modelCalls != 4 {
		t.Fatal("failure survived restart")
	}
	after, err := os.ReadFile(c.path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failure replaced old success")
	}
	f.write(c.path, "{corrupt")
	f.choices(f.support())
	after, err = os.ReadFile(c.path)
	if err != nil || string(after) != "{corrupt" {
		t.Fatal("failed probe rewrote corrupt record")
	}
}

func TestPiProbeCacheWriteFailuresAndFutureWriterDuringProbe(t *testing.T) {
	t.Run("failed directory", func(t *testing.T) {
		f := newPiProbeFixture(t)
		blocked := filepath.Join(f.root, "not-a-directory")
		f.write(blocked, "block")
		t.Setenv("WS_CACHE_HOME", blocked)
		f.probe()
		f.probe()
		if f.modelCalls != 2 || f.versionCalls != 2 {
			t.Fatal("write failure hid a success or created a cache")
		}
		if h := f.choices(f.support()); h.Error != "" {
			t.Fatal("write failure became support error")
		}
	})
	t.Run("failed replace", func(t *testing.T) {
		f := newPiProbeFixture(t)
		for _, kind := range []string{"models", "version"} {
			if err := os.MkdirAll(f.cache(kind).path, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		f.probe()
		entries, err := os.ReadDir(filepath.Dir(f.cache("models").path))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".tmp") {
				t.Fatalf("failed atomic write leaked %s", e.Name())
			}
		}
	})
	t.Run("new writer during probe", func(t *testing.T) {
		f := newPiProbeFixture(t)
		future := `{"schema_version":2,"future":"keep exactly"}`
		f.duringModels = func() { f.write(f.cache("models").path, future) }
		f.duringVersion = func() { f.write(f.cache("version").path, future) }
		f.probe()
		for _, kind := range []string{"models", "version"} {
			raw, err := os.ReadFile(f.cache(kind).path)
			if err != nil || string(raw) != future {
				t.Fatalf("new writer lost: %s %v", raw, err)
			}
		}
	})
}

func TestPiProbeCacheOldWritersAndPrunersThenReopen(t *testing.T) {
	f := newPiProbeFixture(t)
	f.probe()
	before := map[string][]byte{}
	for _, kind := range []string{"models", "version"} {
		raw, err := os.ReadFile(f.cache(kind).path)
		if err != nil {
			t.Fatal(err)
		}
		before[kind] = raw
	}
	dir := filepath.Join(os.Getenv("WS_CACHE_HOME"), "dashboard")
	// The unchanged fixed-shape support writer only replaces support.json.
	s := f.support()
	if err := s.update(func(st *supportState) { st.Last = &Selection{Harness: BackendPi}; st.Stores["fixture"] = "oldreply" }); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := wsdashboard.WriteRegistration(dir, wsdashboard.Registration{ReplyID: "oldreply", Harness: "pi", RegisteredAt: old.Format(time.RFC3339), HeartbeatAt: old.Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	if n := wsdashboard.PruneRegistrations(dir, time.Now(), 24*time.Hour); n != 1 {
		t.Fatalf("old registry prune did not run: %d", n)
	}
	current, stale := filepath.Join(dir, "bin", "current"), filepath.Join(dir, "bin", "stale")
	f.write(current, "current fixture bytes, never executed")
	f.write(stale, "stale fixture bytes, never executed")
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	wsdashboard.PruneStagedExecutables(dir, current)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("old bin prune did not run: %v", err)
	}
	for _, kind := range []string{"models", "version"} {
		raw, err := os.ReadFile(f.cache(kind).path)
		if err != nil || !bytes.Equal(raw, before[kind]) {
			t.Fatalf("old writer/pruner damaged %s cache", kind)
		}
	}
	f.probe()
	if h := f.choices(f.support()); h.Error != "" || len(h.Models) != 3 {
		t.Fatalf("new reopen = %+v", h)
	}
	if f.modelCalls != 1 || f.versionCalls != 1 {
		t.Fatal("old writer/pruner prevented restart reuse")
	}
}
