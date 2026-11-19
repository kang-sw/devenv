package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// sessionChoiceLister bypasses Support's fallback cache as soon as a native
// session reports its authoritative choices. It does no I/O.
type sessionChoiceLister interface {
	sessionChoices() ([]ModelChoice, []string)
}

func (CodexBackend) sessionChoices() ([]ModelChoice, []string) {
	codexChoices.Lock()
	defer codexChoices.Unlock()
	return slices.Clone(codexChoices.models), slices.Clone(codexChoices.efforts)
}

func (b CodexBackend) hostedChoices() ([]ModelChoice, []string, error) {
	if models, efforts := b.sessionChoices(); len(models) > 0 {
		return models, efforts, nil
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return nil, nil, nil
		}
		home = filepath.Join(userHome, ".codex")
	}
	// Support owns the TTL: one read per listing window, no stat/probe on
	// render. Never log raw bytes, paths, parse errors or account metadata.
	raw, _ := os.ReadFile(filepath.Join(home, "models_cache.json"))
	models, efforts := parseCodexCache(raw)
	if reported, levels := b.sessionChoices(); len(reported) > 0 {
		return reported, levels, nil
	}
	return models, efforts, nil
}

func parseCodexCache(raw []byte) ([]ModelChoice, []string) {
	// Decode only the model vocabulary, not identity, etag or account data.
	var cache struct {
		Models []struct {
			Slug string `json:"slug"`
			// Visibility is Codex's picker visibility: "hide" and "none"
			// keep a model out of its own list.
			Visibility string `json:"visibility"`
			Levels     []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if json.Unmarshal(raw, &cache) != nil {
		return nil, nil
	}
	var models []ModelChoice
	var efforts []string
	for _, model := range cache.Models {
		if !modelPattern.MatchString(model.Slug) || model.Visibility == "hide" || model.Visibility == "none" {
			continue
		}
		if !slices.ContainsFunc(models, func(m ModelChoice) bool { return m.ID == model.Slug }) {
			models = append(models, ModelChoice{ID: model.Slug})
		}
		for _, level := range model.Levels {
			if level.Effort != "" && !slices.Contains(efforts, level.Effort) {
				efforts = append(efforts, level.Effort)
			}
		}
	}
	order := []string{"none", "off", "minimal", "low", "medium", "high", "xhigh", "max"}
	slices.SortStableFunc(efforts, func(a, b string) int {
		rank := func(value string) int {
			if i := slices.Index(order, value); i >= 0 {
				return i
			}
			return len(order)
		}
		return rank(a) - rank(b)
	})
	return models, efforts
}

func (AgyBackend) sessionChoices() ([]ModelChoice, []string) {
	agyChoices.mu.Lock()
	defer agyChoices.mu.Unlock()
	efforts := agyChoices.efforts
	if len(efforts) == 0 {
		efforts = agyEfforts
	}
	return slices.Clone(agyChoices.models), slices.Clone(efforts)
}

func (b AgyBackend) hostedChoices(ctx context.Context) ([]ModelChoice, []string, error) {
	models, efforts := b.sessionChoices()
	if len(models) > 0 {
		return models, efforts, nil
	}
	command, pre, err := harnessCommand(b.Command, "agy")
	if err != nil {
		return nil, efforts, fmt.Errorf("agy models: %w", err)
	}
	out, err := runListing(ctx, command, append(pre, "models"), agyEnv(StartSpec{Env: environ()}))
	// A session may have reported while the listing was running, even if
	// that listing failed. Its report wins without waiting out a cache TTL.
	if reported, levels := b.sessionChoices(); len(reported) > 0 {
		return reported, levels, nil
	}
	if err != nil {
		return nil, efforts, fmt.Errorf("agy models: %w", err)
	}
	// A clean exit with no model (agy signed out, say) is a failure, so it
	// takes the short failure TTL and shows, not a cached empty success.
	models = parseAgyModels(out)
	if len(models) == 0 {
		return nil, efforts, errors.New("agy models: the listing named no model; is agy signed in?")
	}
	return models, efforts, nil
}

func parseAgyModels(out []byte) []ModelChoice {
	var models []ModelChoice
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		id, _, ok := strings.Cut(strings.TrimSpace(line), "\t")
		if !ok { // progress and other non-table lines
			continue
		}
		base, _ := agyBaseModel(strings.TrimSpace(id))
		if seen[base] || !modelPattern.MatchString(base) {
			continue
		}
		seen[base] = true
		models = append(models, ModelChoice{ID: base})
	}
	return models
}
