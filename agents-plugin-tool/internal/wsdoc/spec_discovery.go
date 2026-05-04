package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var ticketStemLooseRE = regexp.MustCompile(`\b\d{6}-[\w-]+\b`)

type SpecFindOptions struct {
	Query      string
	SpecStem   string
	TicketStem string
}

type SpecStatusOptions struct {
	SpecStem string
}

type SpecInfo struct {
	Path             string           `json:"path"`
	Filename         string           `json:"filename"`
	Title            string           `json:"title,omitempty"`
	Summary          string           `json:"summary,omitempty"`
	Anchors          []SpecAnchorInfo `json:"anchors,omitempty"`
	TicketRefs       []string         `json:"ticket_refs,omitempty"`
	MarkerContexts   []string         `json:"marker_contexts,omitempty"`
	MatchingSnippets []string         `json:"matching_snippets,omitempty"`
	MatchesSpecStem  bool             `json:"matches_spec_stem,omitempty"`
	MatchesTicketRef bool             `json:"matches_ticket_ref,omitempty"`
}

type SpecAnchorInfo struct {
	SpecStem      string `json:"spec_stem"`
	Line          int    `json:"line"`
	Heading       string `json:"heading,omitempty"`
	MarkerContext string `json:"marker_context,omitempty"`
}

type SpecAnchorStatus struct {
	SpecStem  string           `json:"spec_stem"`
	Locations []SpecAnchorInfo `json:"locations"`
	Files     []SpecInfo       `json:"files"`
}

func SpecsList(root string) ([]SpecInfo, error) {
	return scanSpecs(root)
}

func SpecsFind(root string, opts SpecFindOptions) ([]SpecInfo, error) {
	specs, err := scanSpecs(root)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(opts.Query)
	specStem := strings.TrimSpace(opts.SpecStem)
	ticketStem := strings.TrimSpace(opts.TicketStem)
	if specStem != "" && !specAnchorRE.MatchString("{#"+specStem+"}") {
		return nil, fmt.Errorf("spec_stem must be a spec anchor stem")
	}
	if ticketStem != "" && !ticketStemRE.MatchString(ticketStem) {
		return nil, fmt.Errorf("ticket_stem must be a ticket stem")
	}

	out := []SpecInfo{}
	for _, spec := range specs {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(spec.Path)))
		if err != nil {
			return nil, err
		}
		text := string(raw)
		if specStem != "" && !specHasAnchor(spec, specStem) {
			continue
		}
		if ticketStem != "" && !stringInSlice(ticketStem, spec.TicketRefs) && !strings.Contains(text, ticketStem) {
			continue
		}
		if query != "" {
			haystack := strings.Join([]string{spec.Path, spec.Filename, spec.Title, spec.Summary, text}, "\n")
			if !containsFold(haystack, query) {
				continue
			}
			spec.MatchingSnippets = snippets(text, query, 3)
		}
		if specStem != "" {
			spec.MatchesSpecStem = true
		}
		if ticketStem != "" {
			spec.MatchesTicketRef = true
		}
		out = append(out, spec)
	}
	return out, nil
}

func SpecsStatus(root string, opts SpecStatusOptions) (*SpecAnchorStatus, error) {
	specStem := strings.TrimSpace(opts.SpecStem)
	if specStem == "" {
		return nil, fmt.Errorf("spec_stem is required")
	}
	if !specAnchorRE.MatchString("{#" + specStem + "}") {
		return nil, fmt.Errorf("spec_stem must be a spec anchor stem")
	}
	specs, err := scanSpecs(root)
	if err != nil {
		return nil, err
	}
	status := SpecAnchorStatus{SpecStem: specStem}
	for _, spec := range specs {
		matched := false
		for _, anchor := range spec.Anchors {
			if anchor.SpecStem == specStem {
				status.Locations = append(status.Locations, anchor)
				matched = true
			}
		}
		if matched {
			spec.MatchesSpecStem = true
			status.Files = append(status.Files, spec)
		}
	}
	if len(status.Locations) == 0 {
		return nil, fmt.Errorf("spec anchor not found: %s", specStem)
	}
	sort.Slice(status.Locations, func(i, j int) bool {
		if status.Locations[i].Heading != status.Locations[j].Heading {
			return status.Locations[i].Heading < status.Locations[j].Heading
		}
		return status.Locations[i].Line < status.Locations[j].Line
	})
	return &status, nil
}

func scanSpecs(root string) ([]SpecInfo, error) {
	specRoot := filepath.Join(root, "ai-docs", "spec")
	info, err := os.Stat(specRoot)
	if err != nil {
		return nil, fmt.Errorf("spec directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("spec path is not a directory: %s", specRoot)
	}

	out := []SpecInfo{}
	err = filepath.WalkDir(specRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		spec, err := readSpec(root, path)
		if err != nil {
			return err
		}
		out = append(out, spec)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func readSpec(root, path string) (SpecInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SpecInfo{}, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return SpecInfo{}, err
	}
	text := string(raw)
	fm := frontmatter(path)
	info := SpecInfo{
		Path:     filepath.ToSlash(rel),
		Filename: filepath.Base(path),
	}
	info.Title, _ = fm["title"].(string)
	info.Summary, _ = fm["summary"].(string)
	info.Anchors = specAnchorsInText(text)
	info.TicketRefs = specTicketRefs(fm)
	info.MarkerContexts = specMarkerContexts(text)
	return info, nil
}

func specAnchorsInText(text string) []SpecAnchorInfo {
	lines := strings.Split(text, "\n")
	anchors := []SpecAnchorInfo{}
	heading := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			heading = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		}
		for _, match := range specAnchorRE.FindAllStringSubmatch(line, -1) {
			anchors = append(anchors, SpecAnchorInfo{
				SpecStem:      match[1],
				Line:          i + 1,
				Heading:       cleanHeadingText(heading),
				MarkerContext: markerContext(line),
			})
		}
	}
	return anchors
}

func cleanHeadingText(heading string) string {
	return strings.TrimSpace(specAnchorRE.ReplaceAllString(heading, ""))
}

func specTicketRefs(fm map[string]any) []string {
	refs := map[string]bool{}
	for _, key := range []string{"ticket", "tickets", "feature", "features"} {
		for _, value := range scalarList(fm[key]) {
			for _, match := range ticketStemLooseRE.FindAllString(value, -1) {
				refs[match] = true
			}
		}
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

func specMarkerContexts(text string) []string {
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		context := markerContext(line)
		if context != "" {
			out = append(out, context)
		}
	}
	return out
}

func markerContext(line string) string {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)
	if strings.Contains(trimmed, "🚧") || strings.Contains(lower, "planned") || strings.Contains(lower, "wip") {
		return trimmed
	}
	return ""
}

func specHasAnchor(spec SpecInfo, specStem string) bool {
	for _, anchor := range spec.Anchors {
		if anchor.SpecStem == specStem {
			return true
		}
	}
	return false
}

func stringInSlice(needle string, haystack []string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
