package mcp

import (
	"fmt"
	"strings"
)

// Shared fact-parsing helpers for the route.resolve_* tools and the other
// handlers that accept their typed-fact and format argument shapes.

type factString struct {
	Value   string
	Present bool
	Null    bool
}

func parseRouteFormat(raw any) (string, error) {
	if raw == nil {
		return "text", nil
	}
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("format must be a string")
	}
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "text":
		return "text", nil
	case "json":
		return "json", nil
	default:
		return "", fmt.Errorf("invalid format %q: want one of text, json", s)
	}
}

func parseEnumFact(m map[string]any, name string, allowed []string) (factString, error) {
	f, err := parseObjectString(m, name)
	if err != nil {
		return factString{}, err
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
	return factString{}, fmt.Errorf("invalid %s %q: want one of %s", name, f.Value, strings.Join(allowed, ", "))
}

func parseObjectString(m map[string]any, name string) (factString, error) {
	raw, ok := m[name]
	if !ok {
		return factString{}, nil
	}
	if raw == nil {
		return factString{Present: true, Null: true}, nil
	}
	s, ok := raw.(string)
	if !ok {
		return factString{}, fmt.Errorf("%s must be a string or null", name)
	}
	return factString{Value: strings.TrimSpace(s), Present: true}, nil
}

func factOr(f factString, fallback string) string {
	if !f.Present || f.Null || strings.TrimSpace(f.Value) == "" {
		return fallback
	}
	return normalizeToken(f.Value)
}

func normalizeToken(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
