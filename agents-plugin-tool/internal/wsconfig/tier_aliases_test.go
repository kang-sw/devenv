package wsconfig

import (
	"strings"
	"testing"
)

func TestParseTierAliases(t *testing.T) {
	got, err := ParseTierAliases("  Mockup=PI.deep  quick=claude.small\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["mockup"] != "pi.large" || got["quick"] != "claude.small" {
		t.Fatalf("aliases = %v", got)
	}
	if got, err := ParseTierAliases(""); err != nil || len(got) != 0 {
		t.Fatalf("empty = %v, %v", got, err)
	}
	for raw, want := range map[string]string{
		"mockup":            "is not name=",
		"=pi.small":         "is not name=",
		"a.b=pi.small":      "is not name=",
		"mockup=pi":         "target must be",
		"mockup=pi.huge":    "target must be",
		"mockup=.small":     "target must be",
		"small=pi.large":    "shadow",
		"opus=claude.large": "shadow",
	} {
		if _, err := ParseTierAliases(raw); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseTierAliases(%q) = %v, want an error containing %q", raw, err, want)
		}
	}
}
