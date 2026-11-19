package wsconfig

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// CompactionDefaults are shared by every hosted harness and both readers.
var CompactionDefaults = map[string]string{
	"compaction.lead_advisory": "50%", "compaction.lead_hard": "80%",
	"compaction.worker_advisory": "50%", "compaction.worker_hard": "80%",
}

// ThresholdPattern is the existing percent/token/whichever-first grammar.
const ThresholdPattern = `(?:100|[1-9][0-9]?)%?|(?:[0-9]*[1-9][0-9]*(?:\.[0-9]+)?|0+\.[0-9]*[1-9][0-9]*)[km]|(?:100|[1-9][0-9]?)%?\|(?:[0-9]*[1-9][0-9]*(?:\.[0-9]+)?|0+\.[0-9]*[1-9][0-9]*)[km]|(?:[0-9]*[1-9][0-9]*(?:\.[0-9]+)?|0+\.[0-9]*[1-9][0-9]*)[km]\|(?:100|[1-9][0-9]?)%?`

var thresholdRE = regexp.MustCompile("^(?:" + ThresholdPattern + ")$")

// Threshold holds a percent, a token count, or one of each.
type Threshold struct{ Percent, Tokens float64 }

func ParseThreshold(value string) (Threshold, error) {
	var out Threshold
	if !thresholdRE.MatchString(value) {
		return out, fmt.Errorf("expected a percent (1-100), token count (150k, 1.5m), or one of each joined by |")
	}
	for _, part := range strings.Split(value, "|") {
		scale := float64(0)
		switch part[len(part)-1] {
		case 'k':
			scale = 1000
		case 'm':
			scale = 1000000
		}
		if scale > 0 {
			n, _ := strconv.ParseFloat(part[:len(part)-1], 64)
			out.Tokens = n * scale
			if math.IsInf(out.Tokens, 0) {
				return Threshold{}, fmt.Errorf("token count is too large")
			}
		} else {
			out.Percent, _ = strconv.ParseFloat(strings.TrimSuffix(part, "%"), 64)
		}
	}
	return out, nil
}

// Point is the first threshold inside window; counts beyond it never fire.
func (t Threshold) Point(window float64) float64 {
	point := math.Inf(1)
	if t.Percent > 0 && window > 0 {
		point = window * t.Percent / 100
	}
	if t.Tokens > 0 && window > 0 && t.Tokens <= window {
		point = math.Min(point, t.Tokens)
	}
	return point
}

// Compaction resolves layer-major, unlike the older bucket-major knobs.
// legacy is the descriptor's matching lead key, empty for workers. Builtin
// legacy defaults are deliberately not migration reads.
func (r *Resolver) Compaction(sessionKey, key, harness, legacy string) (ResolvedValue, string, error) {
	def, ok := CompactionDefaults[key]
	if !ok {
		return ResolvedValue{}, "", fmt.Errorf("unknown compaction key %q", key)
	}
	keys := []string{}
	if harness != "" && harness != "*" && harness != "all" {
		keys = append(keys, key+"."+harness)
	}
	keys = append(keys, key)
	if legacy != "" {
		keys = append(keys, legacy)
	}
	if r.sessionR != nil && sessionKey != "" {
		for _, k := range keys {
			if v, ok := r.sessionR.GetOverride(sessionKey, k); ok && v != "" {
				return ResolvedValue{Value: v, Scope: ScopeSession}, k, nil
			}
		}
	}
	project, err := Load(r.opts)
	if err != nil {
		return ResolvedValue{}, "", err
	}
	repo, err := loadRepoConfig(r.opts)
	if err != nil {
		return ResolvedValue{}, "", err
	}
	global, err := loadGlobalConfig(r.opts)
	if err != nil {
		return ResolvedValue{}, "", err
	}
	for _, layer := range []struct {
		scope  Scope
		values map[string]string
	}{{ScopeProject, project.Overrides}, {ScopeRepo, repo.Overrides}, {ScopeGlobal, global.Overrides}} {
		for _, k := range keys {
			if v, ok := layer.values[k]; ok && v != "" {
				return ResolvedValue{Value: v, Scope: layer.scope}, k, nil
			}
		}
	}
	return ResolvedValue{Value: def, Scope: ScopeBuiltin}, key, nil
}
