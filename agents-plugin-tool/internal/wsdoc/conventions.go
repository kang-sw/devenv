package wsdoc

import (
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed conventions/*.md
var conventionFS embed.FS

var conventionCanonicalNames = []string{
	"mental-model-conventions",
	"spec-conventions",
	"ticket-conventions",
}

var conventionAliases = map[string]string{
	"mental-model":             "mental-model-conventions",
	"mental-model-convention":  "mental-model-conventions",
	"mental-model-conventions": "mental-model-conventions",
	"mental-models":            "mental-model-conventions",
	"spec":                     "spec-conventions",
	"spec-convention":          "spec-conventions",
	"spec-conventions":         "spec-conventions",
	"specs":                    "spec-conventions",
	"ticket":                   "ticket-conventions",
	"ticket-convention":        "ticket-conventions",
	"ticket-conventions":       "ticket-conventions",
	"tickets":                  "ticket-conventions",
}

func ReadConvention(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("convention document name is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, string(filepath.Separator)) {
		return "", fmt.Errorf("convention document name must be a bare filename or stem")
	}
	stem := strings.TrimSuffix(strings.TrimSpace(name), ".md")
	canonical, ok := conventionAliases[stem]
	if !ok {
		canonical = stem
	}
	data, err := conventionFS.ReadFile(filepath.ToSlash(filepath.Join("conventions", canonical+".md")))
	if err != nil {
		return "", fmt.Errorf("convention document not found: %s (accepted: %s; aliases: %s)", name, strings.Join(conventionCanonicalNames, ", "), strings.Join(conventionAliasNames(), ", "))
	}
	return string(data), nil
}

func conventionAliasNames() []string {
	aliases := make([]string, 0, len(conventionAliases))
	for alias, canonical := range conventionAliases {
		if alias == canonical {
			continue
		}
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}
