package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func apiListDomains(root string) ([]string, error) {
	entries, err := os.ReadDir(apiDepsDir(root))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	domains := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		domains = append(domains, name)
	}
	sort.Strings(domains)
	return domains, nil
}

func apiDepsDir(root string) string {
	return filepath.Join(root, "ai-docs", ".deps")
}
