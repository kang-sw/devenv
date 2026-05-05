package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func MentalModelsList(root string) (string, error) {
	modelRoot := filepath.Join(root, "ai-docs", "mental-model")
	info, err := os.Stat(modelRoot)
	if err != nil {
		return "", fmt.Errorf("mental-model directory not found: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("mental-model path is not a directory: %s", modelRoot)
	}

	items := []mentalModelItem{}
	if err := filepath.WalkDir(modelRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		fm := frontmatter(path)
		domain, _ := fm["domain"].(string)
		if domain == "" {
			stem := strings.TrimSuffix(filepath.Base(path), ".md")
			if stem == "index" {
				domain = filepath.Base(filepath.Dir(path))
			} else {
				domain = stem
			}
		}
		description, _ := fm["description"].(string)
		sources, _ := fm["sources"].([]string)
		items = append(items, mentalModelItem{
			path:        rel,
			domain:      domain,
			description: description,
			sources:     sources,
		})
		return nil
	}); err != nil {
		return "", err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].path < items[j].path })

	var b strings.Builder
	b.WriteString("mental-models:\n")
	if len(items) == 0 {
		b.WriteString("  (none)\n")
		return b.String(), nil
	}
	for _, item := range items {
		fmt.Fprintf(&b, "  %s  - %s", item.path, item.domain)
		if item.description != "" {
			fmt.Fprintf(&b, "  # %s", item.description)
		}
		b.WriteString("\n")
		if len(item.sources) > 0 {
			fmt.Fprintf(&b, "      sources: %s\n", strings.Join(item.sources, ", "))
		}
	}
	return b.String(), nil
}

type mentalModelItem struct {
	path        string
	domain      string
	description string
	sources     []string
}
