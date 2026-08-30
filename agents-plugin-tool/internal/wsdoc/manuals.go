package wsdoc

import (
	"os"
	"path/filepath"
	"sort"
)

// ManualInfo is one entry under ai-docs/manuals/: a flat, one-line-schema doc
// tier. Unlike MentalModelInfo it carries no domain/sources/spec_refs — the
// manuals schema is deliberately just `summary:`, no applicability predicate.
type ManualInfo struct {
	Path    string `json:"path"`
	Summary string `json:"summary,omitempty"`
}

// ManualsList walks ai-docs/manuals/ (flat glob, no nested subdirectories per
// the ticket's literal `ai-docs/manuals/*.md` wording) and returns one
// ManualInfo per .md file, sorted by path. A manual with no `summary:`
// frontmatter line is still returned with Summary == "" — callers must report
// it, not drop it (see manuals_announcement.go's computeManuals, the sole
// remaining consumer).
//
// Unlike scanMentalModels, a missing ai-docs/manuals/ directory is not an
// error: it returns (nil, nil), the expected steady state until Phase 2
// migrates content into this tier.
func ManualsList(root string) ([]ManualInfo, error) {
	manualRoot := filepath.Join(root, "ai-docs", "manuals")
	entries, err := os.ReadDir(manualRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := []ManualInfo{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(manualRoot, entry.Name())
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		fm := frontmatter(path)
		summary, _ := fm["summary"].(string)
		out = append(out, ManualInfo{
			Path:    filepath.ToSlash(rel),
			Summary: summary,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}
