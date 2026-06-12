package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsrsrc"
)

var ticketRefRE = regexp.MustCompile(`\[(\d{6}-[\w-]+/p\d+)\]`)

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
	renderAIDocs(&b, aiDocs)
	b.WriteString("\n\n")
	if isDir(filepath.Join(aiDocs, "spec")) {
		renderSpecs(&b, filepath.Join(aiDocs, "spec"))
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

func renderAIDocs(b *strings.Builder, aiDocs string) {
	b.WriteString("ai-docs/\n")
	entries := sortedEntries(aiDocs)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "tickets" || name == "spec" {
			continue
		}
		path := filepath.Join(aiDocs, name)
		if entry.IsDir() {
			fmt.Fprintf(b, "  %s/\n", name)
			renderDirTree(b, path, 2)
		} else {
			fmt.Fprintf(b, "  %s\n", name)
		}
	}
}

func renderDirTree(b *strings.Builder, root string, indent int) {
	prefix := strings.Repeat("  ", indent)
	for _, entry := range sortedEntries(root) {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			fmt.Fprintf(b, "%s%s/\n", prefix, entry.Name())
			renderDirTree(b, path, indent+1)
		} else {
			fmt.Fprintf(b, "%s%s\n", prefix, entry.Name())
		}
	}
}

func renderSpecs(b *strings.Builder, specRoot string) {
	b.WriteString("spec:\n")
	renderSpecDir(b, specRoot, 1)
}

func renderSpecDir(b *strings.Builder, root string, indent int) {
	prefix := strings.Repeat("  ", indent)
	for _, entry := range sortedEntries(root) {
		path := filepath.Join(root, entry.Name())
		if entry.IsDir() {
			fmt.Fprintf(b, "%s%s/\n", prefix, entry.Name())
			renderSpecDir(b, path, indent+1)
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
		total, wip, refs := specStats(fm)
		stats := []string{}
		if total > 0 {
			stats = append(stats, fmt.Sprintf("%df", total))
		}
		if wip > 0 {
			wipText := fmt.Sprintf("WIP %d", wip)
			if len(refs) > 0 {
				wipText += " -> " + strings.Join(refs, ", ")
			}
			stats = append(stats, wipText)
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
	}
}

func specStats(fm map[string]any) (int, int, []string) {
	features, _ := fm["features"].([]string)
	refs := []string{}
	wip := 0
	for _, feature := range features {
		if strings.HasPrefix(feature, "🚧") {
			wip++
			matches := ticketRefRE.FindAllStringSubmatch(feature, -1)
			for _, match := range matches {
				refs = append(refs, match[1])
			}
		}
	}
	return len(features), wip, refs
}

func renderTickets(b *strings.Builder, ticketsRoot string) {
	b.WriteString("tickets:\n")
	anyTicket := false
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
			anyTicket = true
			stem := strings.TrimSuffix(entry.Name(), ".md")
			fmt.Fprintf(b, "  [%s] %s\n", status, stem)
			fm := frontmatter(filepath.Join(statusDir, entry.Name()))
			if parent, _ := fm["parent"].(string); parent != "" {
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
