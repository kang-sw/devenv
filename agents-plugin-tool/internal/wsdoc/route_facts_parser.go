package wsdoc

import (
	"fmt"
	"strings"
)

// ValidateRouteFactValues uses the implementation resolver's parser on the
// rows present in a ticket. Missing sections and rows retain their existing
// gate behavior; this check rejects invalid enum values only.
func ValidateRouteFactValues(facts map[string]string) error {
	groups := map[string]any{}
	for key, value := range facts {
		group, name, ok := strings.Cut(key, ".")
		if !ok || (group != "scope" && group != "complexity" && group != "risk") {
			continue
		}
		bucket, ok := groups[group].(map[string]any)
		if !ok {
			bucket = map[string]any{}
			groups[group] = bucket
		}
		bucket[name] = value
	}
	_, err := ParseImplementFacts(groups)
	if err != nil {
		// The resolver's historical message separates the group from the key
		// with "invalid "; name the complete table key at the ready gate.
		message := err.Error()
		if prefix, rest, ok := strings.Cut(message, ".invalid "); ok {
			if name, _, ok := strings.Cut(rest, " "); ok {
				return fmt.Errorf("%s.%s: %w", prefix, name, err)
			}
		}
	}
	return err
}

// FactString preserves presence and null state for route resolution.
type FactString struct {
	Value   string
	Present bool
	Null    bool
}

type Facts struct {
	Scope      ScopeFacts      `json:"scope,omitempty"`
	Complexity ComplexityFacts `json:"complexity,omitempty"`
	Risk       RiskFacts       `json:"risk,omitempty"`
}

type ScopeFacts struct {
	Span            FactString `json:"span,omitempty"`
	Surface         FactString `json:"surface,omitempty"`
	NewPublicSymbol FactString `json:"new_public_symbol,omitempty"`
	NewTypeContract FactString `json:"new_type_contract,omitempty"`
	TestSurface     FactString `json:"test_surface,omitempty"`
}

type ComplexityFacts struct {
	ReusePoints    FactString `json:"reuse_points,omitempty"`
	SideEffectRisk FactString `json:"side_effect_risk,omitempty"`
}

type RiskFacts struct {
	Correctness        FactString `json:"correctness,omitempty"`
	Fit                FactString `json:"fit,omitempty"`
	Test               FactString `json:"test,omitempty"`
	SecurityOrContract FactString `json:"security_or_contract,omitempty"`
}

func ParseImplementFacts(raw any) (Facts, error) {
	if raw == nil {
		return Facts{}, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return Facts{}, fmt.Errorf("facts must be an object")
	}
	var out Facts
	if group, ok := m["scope"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("facts.scope must be an object")
		}
		scope, err := parseScope(gm)
		if err != nil {
			return out, err
		}
		out.Scope = scope
	}
	if group, ok := m["complexity"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("facts.complexity must be an object")
		}
		complexity, err := parseComplexity(gm)
		if err != nil {
			return out, err
		}
		out.Complexity = complexity
	}
	if group, ok := m["risk"]; ok && group != nil {
		gm, ok := group.(map[string]any)
		if !ok {
			return out, fmt.Errorf("facts.risk must be an object")
		}
		risk, err := parseRisk(gm)
		if err != nil {
			return out, err
		}
		out.Risk = risk
	}
	return out, nil
}

func parseScope(m map[string]any) (ScopeFacts, error) {
	var out ScopeFacts
	var err error
	if out.Span, err = parseEnumFact(m, "span", []string{"single-file", "multi-file", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.Surface, err = parseEnumFact(m, "surface", []string{"internal", "public-interface", "cross-module", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.NewPublicSymbol, err = parseEnumFact(m, "new_public_symbol", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.NewTypeContract, err = parseEnumFact(m, "new_type_contract", []string{"yes", "no", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	if out.TestSurface, err = parseEnumFact(m, "test_surface", []string{"none", "existing", "new-files", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.scope.%w", err)
	}
	return out, nil
}

func parseComplexity(m map[string]any) (ComplexityFacts, error) {
	var out ComplexityFacts
	var err error
	if out.ReusePoints, err = parseEnumFact(m, "reuse_points", []string{"confirmed", "unconfirmed", "not-applicable", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.complexity.%w", err)
	}
	if out.SideEffectRisk, err = parseEnumFact(m, "side_effect_risk", []string{"low", "moderate", "high", "unknown"}); err != nil {
		return out, fmt.Errorf("facts.complexity.%w", err)
	}
	return out, nil
}

func parseRisk(m map[string]any) (RiskFacts, error) {
	var out RiskFacts
	var err error
	for _, field := range []struct {
		name string
		dest *FactString
	}{
		{"correctness", &out.Correctness},
		{"fit", &out.Fit},
		{"test", &out.Test},
		{"security_or_contract", &out.SecurityOrContract},
	} {
		if *field.dest, err = parseEnumFact(m, field.name, []string{"low", "moderate", "high", "unknown"}); err != nil {
			return out, fmt.Errorf("facts.risk.%w", err)
		}
	}
	return out, nil
}

func parseEnumFact(m map[string]any, name string, allowed []string) (FactString, error) {
	f, err := parseObjectString(m, name)
	if err != nil {
		return FactString{}, err
	}
	if !f.Present || f.Null || strings.TrimSpace(f.Value) == "" {
		return f, nil
	}
	f.Value = normalizeToken(f.Value)
	for _, value := range allowed {
		if f.Value == value {
			return f, nil
		}
	}
	return FactString{}, fmt.Errorf("invalid %s %q: want one of %s", name, f.Value, strings.Join(allowed, ", "))
}

func parseObjectString(m map[string]any, name string) (FactString, error) {
	raw, ok := m[name]
	if !ok {
		return FactString{}, nil
	}
	if raw == nil {
		return FactString{Present: true, Null: true}, nil
	}
	s, ok := raw.(string)
	if !ok {
		return FactString{}, fmt.Errorf("%s must be a string or null", name)
	}
	return FactString{Value: strings.TrimSpace(s), Present: true}, nil
}

func normalizeToken(raw string) string { return strings.ToLower(strings.TrimSpace(raw)) }
