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
	if isDir(filepath.Join(aiDocs, "tickets")) {
		if err := renderTickets(&b, root); err != nil {
			return "", err
		}
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
		if strings.HasPrefix(name, ".") || name == "tickets" {
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

// renderTickets renders the whole ticket backlog as a single parent-nested
// tree, each node labeled `status/stem` and nested under its `parent:` like a
// filesystem. `related:` edges are not rendered here — they stay reachable
// on-demand via tickets_query. A `.done`/`.dropped` node renders only as a
// dead-parent anchor for a live (idea/todo/ready) descendant; a fully-dead
// subtree (including a placeholder root for a missing parent) is omitted
// entirely. Cycle guard: a malformed `parent:` cycle degrades every node on
// the cycle to a flat root rather than hanging or erroring.
func renderTickets(b *strings.Builder, root string) error {
	b.WriteString("tickets:\n")
	tickets, err := scanTickets(root, ticketScanOptions{IncludeDone: true, IncludeDropped: true})
	if err != nil {
		return err
	}

	// byStem: one entry per stem, first occurrence wins. scanTickets sorts by
	// ticketStatusRank, so a duplicate stem across status directories (an
	// abnormal board) keeps its most-open copy defensively, without crashing.
	byStem := map[string]TicketInfo{}
	order := make([]string, 0, len(tickets))
	for _, ticket := range tickets {
		if _, seen := byStem[ticket.Stem]; seen {
			continue
		}
		byStem[ticket.Stem] = ticket
		order = append(order, ticket.Stem)
	}

	forcedRoot := renderTicketsCycleGuard(byStem, order)

	children := map[string][]string{}
	placeholders := map[string]string{} // placeholder key ("?"+stem) -> missing stem
	rootSet := map[string]bool{}
	for _, stem := range order {
		parent := strings.TrimSpace(byStem[stem].Parent)
		switch {
		case forcedRoot[stem] || parent == "":
			rootSet[stem] = true
		default:
			if _, ok := byStem[parent]; ok {
				children[parent] = append(children[parent], stem)
				continue
			}
			key := "?" + parent
			if _, exists := placeholders[key]; !exists {
				placeholders[key] = parent
				rootSet[key] = true
			}
			children[key] = append(children[key], stem)
		}
	}
	roots := make([]string, 0, len(rootSet))
	for key := range rootSet {
		roots = append(roots, key)
	}
	sort.Strings(roots)

	memo := map[string]bool{}
	var shouldRender func(key string) bool
	shouldRender = func(key string) bool {
		if v, ok := memo[key]; ok {
			return v
		}
		memo[key] = false // guard: the tree is acyclic by construction
		result := false
		if _, isPlaceholder := placeholders[key]; !isPlaceholder && isLiveStatus(byStem[key].Status) {
			result = true
		} else {
			for _, child := range children[key] {
				if shouldRender(child) {
					result = true
					break
				}
			}
		}
		memo[key] = result
		return result
	}

	printed := false
	var render func(key string, depth int)
	render = func(key string, depth int) {
		if !shouldRender(key) {
			return
		}
		label := renderTicketsLabel(key, byStem, placeholders)
		fmt.Fprintf(b, "%s%s\n", strings.Repeat("  ", depth), label)
		printed = true
		childs := append([]string(nil), children[key]...)
		sort.Strings(childs)
		for _, child := range childs {
			render(child, depth+1)
		}
	}
	for _, key := range roots {
		render(key, 1)
	}
	if !printed {
		b.WriteString("  (none)\n")
	}
	return nil
}

// renderTicketsCycleGuard runs a three-color (white/gray/black) walk over
// byStem's functional `parent:` graph (at most one outgoing edge per node) and
// returns the set of stems that must render as flat roots because they sit on
// a `parent:` cycle. A gray stem is on the current walk's path; hitting a gray
// stem again means every stem from that position to the end of the path is
// part of the cycle and becomes a forced root. Hitting an unresolved parent or
// an already-black stem ends the walk normally (no forced roots on that path).
func renderTicketsCycleGuard(byStem map[string]TicketInfo, order []string) map[string]bool {
	const (
		white = iota
		gray
		black
	)
	color := map[string]int{}
	forcedRoot := map[string]bool{}

	for _, start := range order {
		if color[start] != white {
			continue
		}
		var path []string
		current := start
		for {
			color[current] = gray
			path = append(path, current)
			parent := strings.TrimSpace(byStem[current].Parent)
			if parent == "" {
				break
			}
			if _, ok := byStem[parent]; !ok {
				break
			}
			switch color[parent] {
			case white:
				current = parent
				continue
			case gray:
				for i, stem := range path {
					if stem == parent {
						for _, cycleStem := range path[i:] {
							forcedRoot[cycleStem] = true
						}
						break
					}
				}
			}
			// black (already resolved by an earlier walk) or gray (cycle,
			// handled above): stop extending this path either way.
			break
		}
		for _, stem := range path {
			color[stem] = black
		}
	}
	return forcedRoot
}

func isLiveStatus(status string) bool {
	return status == "idea" || status == "todo" || status == "ready"
}

func renderTicketsLabel(key string, byStem map[string]TicketInfo, placeholders map[string]string) string {
	if missing, ok := placeholders[key]; ok {
		return "?/" + missing
	}
	ticket := byStem[key]
	return strings.TrimPrefix(ticket.Status, ".") + "/" + ticket.Stem
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
