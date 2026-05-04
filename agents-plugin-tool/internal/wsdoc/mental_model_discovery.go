package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var specStemLooseRE = regexp.MustCompile(`(?:#|\{#)([0-9]{6}-[a-z0-9-]+)\}?`)

type MentalModelFindOptions struct {
	Query    string
	SpecStem string
	Domain   string
}

type MentalModelStatusOptions struct {
	Domain string
	Path   string
}

type MentalModelInfo struct {
	Path             string   `json:"path"`
	Domain           string   `json:"domain"`
	Description      string   `json:"description,omitempty"`
	Sources          []string `json:"sources,omitempty"`
	SpecRefs         []string `json:"spec_refs,omitempty"`
	AncestorHints    []string `json:"ancestor_hints,omitempty"`
	IndexHints       []string `json:"index_hints,omitempty"`
	MatchingSnippets []string `json:"matching_snippets,omitempty"`
	MatchesSpecStem  bool     `json:"matches_spec_stem,omitempty"`
	MatchesDomain    bool     `json:"matches_domain,omitempty"`
}

func MentalModelsFind(root string, opts MentalModelFindOptions) ([]MentalModelInfo, error) {
	models, err := scanMentalModels(root)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(opts.Query)
	specStem := strings.TrimSpace(opts.SpecStem)
	domain := strings.TrimSpace(opts.Domain)
	if specStem != "" && !specAnchorRE.MatchString("{#"+specStem+"}") {
		return nil, fmt.Errorf("spec_stem must be a spec anchor stem")
	}

	out := []MentalModelInfo{}
	for _, model := range models {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(model.Path)))
		if err != nil {
			return nil, err
		}
		text := string(raw)
		if domain != "" && !strings.EqualFold(model.Domain, domain) {
			continue
		}
		if specStem != "" && !stringInSlice(specStem, model.SpecRefs) && !strings.Contains(text, specStem) {
			continue
		}
		if query != "" {
			haystack := strings.Join([]string{model.Path, model.Domain, model.Description, strings.Join(model.Sources, "\n"), text}, "\n")
			if !containsFold(haystack, query) {
				continue
			}
			model.MatchingSnippets = snippets(text, query, 3)
		}
		if specStem != "" {
			model.MatchesSpecStem = true
		}
		if domain != "" {
			model.MatchesDomain = true
		}
		out = append(out, model)
	}
	return out, nil
}

func MentalModelsStatus(root string, opts MentalModelStatusOptions) ([]MentalModelInfo, error) {
	domain := strings.TrimSpace(opts.Domain)
	path := strings.TrimSpace(opts.Path)
	if domain == "" && path == "" {
		return nil, fmt.Errorf("domain or path is required")
	}
	if path != "" && !isMentalModelRelPath(path) {
		return nil, fmt.Errorf("path must be under ai-docs/mental-model")
	}
	models, err := scanMentalModels(root)
	if err != nil {
		return nil, err
	}
	out := []MentalModelInfo{}
	for _, model := range models {
		if domain != "" && !strings.EqualFold(model.Domain, domain) {
			continue
		}
		if path != "" && filepath.ToSlash(path) != model.Path {
			continue
		}
		if domain != "" {
			model.MatchesDomain = true
		}
		out = append(out, model)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("mental-model not found")
	}
	return out, nil
}

func scanMentalModels(root string) ([]MentalModelInfo, error) {
	modelRoot := filepath.Join(root, "ai-docs", "mental-model")
	info, err := os.Stat(modelRoot)
	if err != nil {
		return nil, fmt.Errorf("mental-model directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("mental-model path is not a directory: %s", modelRoot)
	}

	out := []MentalModelInfo{}
	err = filepath.WalkDir(modelRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		model, err := readMentalModel(root, path)
		if err != nil {
			return err
		}
		out = append(out, model)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func readMentalModel(root, path string) (MentalModelInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return MentalModelInfo{}, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return MentalModelInfo{}, err
	}
	fm := frontmatter(path)
	model := MentalModelInfo{
		Path:          filepath.ToSlash(rel),
		AncestorHints: mentalModelAncestorHints(root, path),
		IndexHints:    mentalModelIndexHints(root, path),
	}
	model.Domain, _ = fm["domain"].(string)
	if model.Domain == "" {
		model.Domain = inferredMentalModelDomain(path)
	}
	model.Description, _ = fm["description"].(string)
	model.Sources = append(scalarList(fm["source"]), scalarList(fm["sources"])...)
	model.SpecRefs = mentalModelSpecRefs(fm, string(raw))
	return model, nil
}

func inferredMentalModelDomain(path string) string {
	stem := strings.TrimSuffix(filepath.Base(path), ".md")
	if stem == "index" {
		return filepath.Base(filepath.Dir(path))
	}
	return stem
}

func mentalModelSpecRefs(fm map[string]any, text string) []string {
	refs := map[string]bool{}
	for _, key := range []string{"spec", "specs", "source", "sources"} {
		for _, value := range scalarList(fm[key]) {
			collectSpecRefs(refs, value)
		}
	}
	collectSpecRefs(refs, text)
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func collectSpecRefs(refs map[string]bool, text string) {
	for _, match := range specStemLooseRE.FindAllStringSubmatch(text, -1) {
		refs[match[1]] = true
	}
	if specAnchorRE.MatchString("{#" + text + "}") {
		refs[text] = true
	}
}

func mentalModelAncestorHints(root, path string) []string {
	modelRoot := filepath.Join(root, "ai-docs", "mental-model")
	dir := filepath.Dir(path)
	hints := []string{}
	for {
		if dir == modelRoot || dir == "." || dir == string(filepath.Separator) {
			break
		}
		rel, err := filepath.Rel(root, dir)
		if err == nil {
			hints = append(hints, filepath.ToSlash(rel))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	sort.Strings(hints)
	return hints
}

func mentalModelIndexHints(root, path string) []string {
	modelRoot := filepath.Join(root, "ai-docs", "mental-model")
	dir := filepath.Dir(path)
	hints := []string{}
	for {
		index := filepath.Join(dir, "index.md")
		if index != path {
			if _, err := os.Stat(index); err == nil {
				rel, err := filepath.Rel(root, index)
				if err == nil {
					hints = append(hints, filepath.ToSlash(rel))
				}
			}
		}
		if dir == modelRoot || dir == "." || dir == string(filepath.Separator) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	sort.Strings(hints)
	return hints
}

func isMentalModelRelPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return strings.HasPrefix(clean, "ai-docs/mental-model/") && !strings.Contains(clean, "../")
}
