package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func emptyHostedChoices(t *testing.T) {
	t.Helper()
	codexChoices.Lock()
	cm, ce := codexChoices.models, codexChoices.efforts
	codexChoices.models, codexChoices.efforts = nil, nil
	codexChoices.Unlock()
	agyChoices.mu.Lock()
	am, ae := agyChoices.models, agyChoices.efforts
	agyChoices.models, agyChoices.efforts = nil, nil
	agyChoices.mu.Unlock()
	t.Cleanup(func() {
		codexChoices.Lock()
		codexChoices.models, codexChoices.efforts = cm, ce
		codexChoices.Unlock()
		agyChoices.mu.Lock()
		agyChoices.models, agyChoices.efforts = am, ae
		agyChoices.mu.Unlock()
	})
}

const syntheticCodexCache = `{
 "fetched_at":"synthetic-timestamp", "etag":"SYNTHETIC-ETAG", "client_version":"synthetic-version",
 "identity":{"account_id":"SYNTHETIC-PRIVATE-ACCOUNT", "plan":"SYNTHETIC-PRIVATE-PLAN"},
 "models":[
  {"slug":"fixture-reasoner","display_name":"Fixture Reasoner","supported_reasoning_levels":[{"effort":"high"},{"effort":"low"}]},
  {"slug":"fixture-plain","visibility":"list","supported_reasoning_levels":[]},
  {"slug":"fixture-hidden","visibility":"hide","supported_reasoning_levels":[{"effort":"max"}]},
  {"slug":"fixture-unlisted","visibility":"none","supported_reasoning_levels":[{"effort":"minimal"}]},
  {"slug":"fixture-second","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"medium"},{"effort":"low"},{"effort":""}]},
  {"slug":"fixture-reasoner"}, {"slug":"","supported_reasoning_levels":[{"effort":"invalid"}]}
 ]
}`

func TestHostedCodexCacheChoices(t *testing.T) {
	emptyHostedChoices(t)
	for _, fallback := range []bool{false, true} {
		t.Run(map[bool]string{false: "CODEX_HOME", true: "default-home"}[fallback], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			cacheDir := home
			if fallback {
				t.Setenv("CODEX_HOME", "")
				cacheDir = filepath.Join(home, ".codex")
				if err := os.MkdirAll(cacheDir, 0o700); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("CODEX_HOME", cacheDir)
			}
			path := filepath.Join(cacheDir, "models_cache.json")
			b := CodexBackend{}
			checkEmpty := func() {
				t.Helper()
				models, efforts, err := b.Choices(context.Background())
				if err != nil || len(models) != 0 || len(efforts) != 0 {
					t.Fatalf("empty cache: %v %v %v", models, efforts, err)
				}
			}
			checkEmpty() // missing is not an error
			if err := os.WriteFile(path, []byte(`{"models":broken`), 0o600); err != nil {
				t.Fatal(err)
			}
			checkEmpty()
			if err := os.WriteFile(path, []byte(syntheticCodexCache), 0o600); err != nil {
				t.Fatal(err)
			}
			models, efforts, err := b.Choices(context.Background())
			want := []ModelChoice{{ID: "fixture-reasoner"}, {ID: "fixture-plain"}, {ID: "fixture-second"}}
			if err != nil || !reflect.DeepEqual(models, want) || !slices.Equal(efforts, []string{"low", "medium", "high", "xhigh"}) {
				t.Fatalf("cache choices: %v %v %v", models, efforts, err)
			}
			raw, err := json.Marshal(struct {
				Models  []ModelChoice
				Efforts []string
			}{models, efforts})
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"identity", "etag", "account", "plan", "SYNTHETIC", "fetched_at", "client_version"} {
				if strings.Contains(string(raw), secret) {
					t.Fatalf("private metadata %q leaked", secret)
				}
			}
		})
	}
}

const syntheticAgyModels = "Fetching available models...\nfixture-gemini-high\tFixture Gemini (High)\nfixture-gemini-low\tFixture Gemini (Low)\nfixture-gemini-medium\tFixture Gemini (Medium)\nfixture-orchestrator\tFixture Orchestrator\r\nfixture-gemini-xhigh\tFixture Gemini (XHigh)\ninvalid model\tInvalid\n\n"

func TestHostedAgyListingCommandAndEnvironment(t *testing.T) {
	emptyHostedChoices(t)
	oldListing, oldEnv := runListing, environ
	t.Cleanup(func() { runListing, environ = oldListing, oldEnv })
	environ = func() []string {
		return []string{"PATH=/fixture/bin", "AGY_HOME=fixture-home", "CLAUDE_SESSION=x", "CODEX_HOME=x", "PI_SESSION=x", "WS_MCP_SESSION_TOKEN=x"}
	}
	calls := 0
	runListing = func(ctx context.Context, command string, args, env []string) ([]byte, error) {
		calls++
		if command != "fixture-agy" || !slices.Equal(args, []string{"models"}) {
			t.Fatalf("command: %s %v", command, args)
		}
		if !slices.Equal(env, agyEnv(StartSpec{Env: environ()})) || !slices.Contains(env, "AGY_HOME=fixture-home") {
			t.Fatalf("listing env differs from session env: %v", env)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > choicesTimeout {
			t.Fatal("listing is not bounded by choicesTimeout")
		}
		return []byte(syntheticAgyModels), nil
	}
	now := time.Now()
	s := hostedChoicesSupport(t, &now)
	h := harnessByName(t, s.Choices(context.Background()), BackendAgy)
	if h.Error != "" || !reflect.DeepEqual(h.Models, []ModelChoice{{ID: "fixture-gemini"}, {ID: "fixture-orchestrator"}}) || !slices.Equal(h.Efforts, agyEfforts) {
		t.Fatalf("agy listing: %+v", h)
	}
	if !slices.Contains(h.PermissionModes, "bypass") {
		t.Fatal("bypass not offered")
	}
	s.Choices(context.Background())
	if calls != 1 {
		t.Fatalf("%d listing calls in a cache window", calls)
	}
	now = now.Add(choicesTTL)
	s.Choices(context.Background())
	if calls != 2 {
		t.Fatalf("%d listing calls after expiry", calls)
	}
}

func hostedChoicesSupport(t *testing.T, now *time.Time) *Support {
	t.Helper()
	stubLookPath(t)
	// Every Codex file read in these tests is confined to synthetic state.
	t.Setenv("CODEX_HOME", t.TempDir())
	r := NewRunner(Options{Dir: t.TempDir(), Config: testConfig(t), Backends: map[string]BackendRegistration{
		BackendCodex: {Backend: CodexBackend{}, LeadProfile: LeadProfileCodex, PermissionModes: CodexPermissionModes},
		BackendAgy:   {Backend: AgyBackend{Command: "fixture-agy"}, LeadProfile: LeadProfileAgy, PermissionModes: AgyPermissionModes},
	}})
	return NewSupport(SupportOptions{Runner: r, Dir: t.TempDir(), Now: func() time.Time { return *now }})
}

func TestHostedChoicesCacheAndSessionPrecedence(t *testing.T) {
	emptyHostedChoices(t)
	now := time.Now()
	s := hostedChoicesSupport(t, &now)
	path := filepath.Join(os.Getenv("CODEX_HOME"), "models_cache.json")
	if err := os.WriteFile(path, []byte(syntheticCodexCache), 0o600); err != nil {
		t.Fatal(err)
	}
	listing := stubListing(t, "", errors.New("synthetic listing failure"))
	first := s.Choices(context.Background())
	if h := harnessByName(t, first, BackendAgy); h.Error == "" || !h.Available || !slices.Equal(h.Efforts, agyEfforts) {
		t.Fatalf("failure: %+v", h)
	}
	if h := harnessByName(t, first, BackendCodex); h.Error != "" || len(h.Models) != 3 {
		t.Fatalf("other harness affected: %+v", h)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	cached := s.Choices(context.Background())
	if h := harnessByName(t, cached, BackendCodex); len(h.Models) != 3 {
		t.Fatal("Codex cache reread inside window")
	}
	if listing.count() != 1 {
		t.Fatal("failed agy listing repeated inside failure window")
	}
	now = now.Add(choicesFailTTL)
	s.Choices(context.Background())
	if listing.count() != 2 {
		t.Fatal("failed agy listing did not expire")
	}
	now = now.Add(choicesTTL)
	if h := harnessByName(t, s.Choices(context.Background()), BackendCodex); len(h.Models) != 0 || h.Error != "" {
		t.Fatal("Codex cache did not expire")
	}

	// Session reports immediately replace fresh empty/success/failure cache
	// entries, without another read or process spawn.
	codexModels(json.RawMessage(`{"data":[{"model":"session-codex","supportedReasoningEfforts":[{"reasoningEffort":"low"}]}]}`))
	m := newAgyMapper("")
	m.configOptions(json.RawMessage(`{"configOptions":[{"id":"model","options":[{"value":"session-agy-high"}]},{"id":"thought_level","options":[{"value":"high"}]}]}`))
	before := listing.count()
	ans := s.Choices(context.Background())
	for backend, want := range map[string]string{BackendCodex: "session-codex", BackendAgy: "session-agy"} {
		h := harnessByName(t, ans, backend)
		if h.Error != "" || len(h.Models) != 1 || h.Models[0].ID != want {
			t.Fatalf("session report did not win: %+v", h)
		}
	}
	if h := harnessByName(t, ans, BackendAgy); !slices.Equal(h.Efforts, []string{"high"}) {
		t.Fatalf("session efforts: %v", h.Efforts)
	}
	if listing.count() != before {
		t.Fatal("session-backed choices spawned a listing")
	}
}

func TestHostedAgySessionReportDuringListingWins(t *testing.T) {
	emptyHostedChoices(t)
	listing := stubListing(t, syntheticAgyModels, errors.New("synthetic failure"))
	listing.during = func() {
		m := newAgyMapper("")
		m.configOptions(json.RawMessage(`{"configOptions":[{"id":"model","options":[{"value":"session-winner-low"}]}]}`))
	}
	models, _, err := (AgyBackend{Command: "fixture-agy"}).Choices(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "session-winner" {
		t.Fatalf("report lost: %v %v", models, err)
	}
}

// TestHostedAgyEmptyListingFails: a listing that exits cleanly but names no
// model (agy signed out) is a failure: it shows and expires after
// choicesFailTTL instead of caching an empty success for choicesTTL.
func TestHostedAgyEmptyListingFails(t *testing.T) {
	emptyHostedChoices(t)
	now := time.Now()
	s := hostedChoicesSupport(t, &now)
	listing := stubListing(t, "Fetching available models...\n", nil)
	if h := harnessByName(t, s.Choices(context.Background()), BackendAgy); h.Error == "" || len(h.Models) != 0 || !slices.Equal(h.Efforts, agyEfforts) {
		t.Fatalf("empty listing: %+v", h)
	}
	now = now.Add(choicesFailTTL)
	s.Choices(context.Background())
	if listing.count() != 2 {
		t.Fatalf("%d listings: an empty listing was cached past choicesFailTTL", listing.count())
	}
}
