package wsdoc

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

func ProjectTree(root string) (string, error) {
	aiDocs := filepath.Join(root, "ai-docs")
	info, err := os.Stat(aiDocs)
	if err != nil {
		return "", fmt.Errorf("ai-docs not found: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("ai-docs is not a directory: %s", aiDocs)
	}

	var b strings.Builder
	ignored := gitIgnoreMatcher(root)
	renderAIDocs(&b, aiDocs, ignored)
	b.WriteString("\n\n")
	if isDir(filepath.Join(aiDocs, "spec")) {
		renderSpecs(&b, root, filepath.Join(aiDocs, "spec"))
		b.WriteString("\n\n")
	}
	if isDir(filepath.Join(aiDocs, "tickets")) {
		renderTickets(&b, filepath.Join(aiDocs, "tickets"))
	}
	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

// ReadInfra returns an infra document body by bare stem, loaded from the rsrc
// tree (260611 Phase 6b retired the wsprompt go:embed bundle). Path-escaping
// names are rejected so callers cannot read outside the rsrc root.
func ReadInfra(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("infra document name is required")
	}
	stem := strings.TrimSuffix(name, ".md")
	if stem == "" || stem == "." || stem == ".." ||
		strings.ContainsAny(stem, `/\`) || strings.Contains(stem, "..") {
		return "", fmt.Errorf("infra document name must be a bare filename or stem")
	}
	root, err := wsrsrc.ResolveRoot()
	if err != nil {
		return "", fmt.Errorf("resolve rsrc root: %w", err)
	}
	pb, err := wsrsrc.Load(root, stem, "", nil)
	if err != nil {
		return "", err
	}
	return pb.Body, nil
}

func renderAIDocs(b *strings.Builder, aiDocs string, ignored func(string) bool) {
	b.WriteString("ai-docs/\n")
	entries := sortedEntries(aiDocs)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "tickets" || name == "spec" {
			continue
		}
		path := filepath.Join(aiDocs, name)
		if ignored(path) {
			continue
		}
		if entry.IsDir() {
			fmt.Fprintf(b, "  %s/\n", name)
			renderDirTree(b, path, 2, ignored)
		} else {
			fmt.Fprintf(b, "  %s\n", name)
		}
	}
}

func renderDirTree(b *strings.Builder, root string, indent int, ignored func(string) bool) {
	prefix := strings.Repeat("  ", indent)
	for _, entry := range sortedEntries(root) {
		path := filepath.Join(root, entry.Name())
		if ignored(path) {
			continue
		}
		if entry.IsDir() {
			fmt.Fprintf(b, "%s%s/\n", prefix, entry.Name())
			renderDirTree(b, path, indent+1, ignored)
		} else {
			fmt.Fprintf(b, "%s%s\n", prefix, entry.Name())
		}
	}
}

func gitIgnoreMatcher(repoRoot string) func(string) bool {
	if err := exec.Command("git", "-C", repoRoot, "rev-parse", "--is-inside-work-tree").Run(); err != nil {
		return func(string) bool { return false }
	}
	cache := map[string]bool{}
	return func(path string) bool {
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return false
		}
		rel = filepath.ToSlash(rel)
		if ignored, ok := cache[rel]; ok {
			return ignored
		}
		err = exec.Command("git", "-C", repoRoot, "check-ignore", "-q", "--", rel).Run()
		ignored := err == nil
		cache[rel] = ignored
		return ignored
	}
}

// renderSpecs takes both the repository root and the spec directory: the legacy
// planned-marker advisory resolves against live tickets, which live outside the
// spec tree. A `needed` pre-pass runs first, matching applyLegacyMarkerAdvisories
// in spec_discovery.go: project_tree is the session-bootstrap tool, so a project
// with zero markers must not pay a full live-ticket scan on every session start.
// When the pre-pass finds markers the resolver is built exactly once per call.
func renderSpecs(b *strings.Builder, repoRoot, specRoot string) {
	b.WriteString("spec:\n")
	markers := scanLegacyMarkersUnderSpecRoot(specRoot)
	var resolver *legacyMarkerResolver
	if len(markers) > 0 {
		resolver = newLegacyMarkerResolver(repoRoot)
	}
	renderSpecDir(b, repoRoot, specRoot, 1, markers, resolver)
}

// scanLegacyMarkersUnderSpecRoot reads each spec document once and keys the
// markers it carries by absolute path. Marker-free files are absent from the
// map, so an empty map is the `needed == false` answer. Read failures are
// skipped: the advisory never blocks tree rendering.
func scanLegacyMarkersUnderSpecRoot(specRoot string) map[string][]legacyMarker {
	out := map[string][]legacyMarker{}
	_ = filepath.WalkDir(specRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil //nolint:nilerr // advisory scan degrades to no-op
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if found := legacyMarkerLines(string(raw)); len(found) > 0 {
			out[path] = found
		}
		return nil
	})
	return out
}

func renderSpecDir(b *strings.Builder, repoRoot, dir string, indent int, markers map[string][]legacyMarker, resolver *legacyMarkerResolver) {
	prefix := strings.Repeat("  ", indent)
	for _, entry := range sortedEntries(dir) {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			fmt.Fprintf(b, "%s%s/\n", prefix, entry.Name())
			renderSpecDir(b, repoRoot, path, indent+1, markers, resolver)
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		fm := frontmatter(path)
		title, _ := fm["title"].(string)
		if title == "" {
			title, _ = fm["summary"].(string)
		}
		stats := []string{}
		if total := specStats(fm); total > 0 {
			stats = append(stats, fmt.Sprintf("%df", total))
		}
		titlePart := ""
		if title != "" {
			titlePart = "  - " + title
		}
		statsPart := ""
		if len(stats) > 0 {
			statsPart = "  [" + strings.Join(stats, ", ") + "]"
		}
		fmt.Fprintf(b, "%s%s%s%s\n", prefix, entry.Name(), titlePart, statsPart)
		if advisory := legacyMarkerAdvisoryFor(repoRoot, path, markers[path], resolver); advisory != "" {
			fmt.Fprintf(b, "%s  legacy-marker: %s\n", prefix, advisory)
		}
	}
}

// legacyMarkerAdvisoryFor uses the pre-pass's marker set rather than reusing
// specStats: specStats reads `features:` frontmatter, which no spec file in
// this corpus declares, so a frontmatter-only check detects nothing. A missing
// resolver or an unresolvable relative path yields no advisory — the note never
// blocks tree rendering.
func legacyMarkerAdvisoryFor(repoRoot, path string, markers []legacyMarker, resolver *legacyMarkerResolver) string {
	if resolver == nil || len(markers) == 0 {
		return ""
	}
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return ""
	}
	return resolver.advise(filepath.ToSlash(rel), markers)
}

// specStats counts the `features:` frontmatter entries a spec declares. The
// planned/WIP half was retired with the `🚧` marker mechanism; the plain count
// stays because downstream projects still maintain `features:` frontmatter.
func specStats(fm map[string]any) int {
	features, _ := fm["features"].([]string)
	return len(features)
}

func renderTickets(b *strings.Builder, ticketsRoot string) {
	b.WriteString("tickets:\n")
	anyTicket := false
	orphanIdea := 0
	for _, status := range []string{"ready", "todo", "idea"} {
		statusDir := filepath.Join(ticketsRoot, status)
		if !isDir(statusDir) {
			continue
		}
		entries := sortedEntries(statusDir)
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			stem := strings.TrimSuffix(entry.Name(), ".md")
			fm := frontmatter(filepath.Join(statusDir, entry.Name()))
			parent, _ := fm["parent"].(string)
			anyTicket = true
			if status == "idea" && parent == "" {
				orphanIdea++
				continue
			}
			fmt.Fprintf(b, "  [%s] %s\n", status, stem)
			if parent != "" {
				fmt.Fprintf(b, "      parent: %s%s\n", parent, titleSuffix(parent, ticketsRoot))
			}
			if related, _ := fm["related"].(map[string]string); len(related) > 0 {
				keys := make([]string, 0, len(related))
				for key := range related {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					note := related[key]
					parts := []string{}
					if note != "" {
						parts = append(parts, note)
					}
					if title := ticketTitle(key, ticketsRoot); title != "" {
						parts = append(parts, title)
					}
					suffix := ""
					if len(parts) > 0 {
						suffix = "  # " + strings.Join(parts, " · ")
					}
					fmt.Fprintf(b, "      related: %s%s\n", key, suffix)
				}
			}
		}
	}
	if orphanIdea > 0 {
		fmt.Fprintf(b, "  idea: %d orphan hidden — tickets.query statuses=idea to view\n", orphanIdea)
	}
	if !anyTicket {
		b.WriteString("  (none)\n")
	}
}

func titleSuffix(stem, ticketsRoot string) string {
	title := ticketTitle(stem, ticketsRoot)
	if title == "" {
		return ""
	}
	return "  # " + title
}

func ticketTitle(stem, ticketsRoot string) string {
	for _, status := range []string{"ready", "todo", "idea", "wip", ".done", ".dropped"} {
		path := filepath.Join(ticketsRoot, status, stem+".md")
		if _, err := os.Stat(path); err == nil {
			title, _ := frontmatter(path)["title"].(string)
			return title
		}
	}
	return ""
}

func sortedEntries(root string) []os.DirEntry {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	return entries
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
