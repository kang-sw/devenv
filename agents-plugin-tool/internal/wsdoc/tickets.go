package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var ticketStemRE = regexp.MustCompile(`^\d{6}-[\w-]+$`)

type TicketListOptions struct {
	Statuses       []string
	IncludeDone    bool
	IncludeDropped bool
}

type TicketFindOptions struct {
	Statuses           []string
	IncludeDone        bool
	IncludeDropped     bool
	Query              string
	TicketStem         string
	MentionsTicketStem string
	// Resolve marks this call as a resolution query rather than a discovery
	// one: under an active sparse-checkout scope the board is completed from
	// the index, so a stem hidden by this worktree still resolves. It is a
	// property of the call, not of the function — references.trace shares this
	// entry point with discovery callers and must be able to ask for the whole
	// board through it.
	Resolve bool
}

type TicketStatusOptions struct {
	TicketStem     string
	IncludeDone    bool
	IncludeDropped bool
	// Resolve — see TicketFindOptions.Resolve.
	Resolve bool
}

type TicketInfo struct {
	Stem               string            `json:"stem"`
	Path               string            `json:"path"`
	Status             string            `json:"status"`
	Title              string            `json:"title,omitempty"`
	Parent             string            `json:"parent,omitempty"`
	Related            map[string]string `json:"related,omitempty"`
	Specs              []string          `json:"specs,omitempty"`
	SpecRemoves        []string          `json:"spec_removes,omitempty"`
	Plans              []string          `json:"plans,omitempty"`
	Skeletons          []string          `json:"skeletons,omitempty"`
	Completed          string            `json:"completed,omitempty"`
	ResultPresent      bool              `json:"result_present"`
	UnresolvedPhases   []string          `json:"unresolved_phases,omitempty"`
	Phases             []TicketPhase     `json:"phases,omitempty"`
	MatchingSnippets   []string          `json:"matching_snippets,omitempty"`
	MentionsTicketStem bool              `json:"mentions_ticket_stem,omitempty"`
	// Hidden marks an entry sourced from the git index with no file on disk,
	// i.e. a ticket this worktree's sparse-checkout scope excludes. It is only
	// ever set on a resolution-mode call; discovery calls stay filesystem-only.
	Hidden bool `json:"hidden,omitempty"`
}

type TicketPhase struct {
	Heading       string `json:"heading"`
	ResultPresent bool   `json:"result_present"`
}

func TicketsList(root string, opts TicketListOptions) ([]TicketInfo, error) {
	return scanTickets(root, ticketScanOptions{
		Statuses:       opts.Statuses,
		IncludeDone:    opts.IncludeDone,
		IncludeDropped: opts.IncludeDropped,
	})
}

func TicketsFind(root string, opts TicketFindOptions) ([]TicketInfo, error) {
	tickets, bodies, err := scanTicketsWithBodies(root, ticketScanOptions{
		Statuses:       opts.Statuses,
		IncludeDone:    opts.IncludeDone,
		IncludeDropped: opts.IncludeDropped,
		Resolve:        resolveModeFor(opts.Resolve),
	})
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(opts.Query)
	ticketStem := strings.TrimSpace(opts.TicketStem)
	mentions := strings.TrimSpace(opts.MentionsTicketStem)
	if ticketStem != "" && !ticketStemRE.MatchString(ticketStem) {
		return nil, fmt.Errorf("ticket_stem must be a ticket stem")
	}
	if mentions != "" && !ticketStemRE.MatchString(mentions) {
		return nil, fmt.Errorf("mentions_ticket_stem must be a ticket stem")
	}

	out := []TicketInfo{}
	for _, ticket := range tickets {
		// A hidden ticket has no file to read; its body comes from the index
		// blob the scan already batched. Skipping it here instead would break
		// references.trace's spec branch, which matches spec stems against
		// ticket bodies.
		text, ok := bodies[ticket.Path]
		if !ok {
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ticket.Path)))
			if err != nil {
				return nil, err
			}
			text = string(raw)
		}
		if ticketStem != "" && ticket.Stem != ticketStem {
			continue
		}
		if mentions != "" && !strings.Contains(text, mentions) {
			continue
		}
		if query != "" {
			haystack := strings.Join([]string{ticket.Stem, ticket.Path, ticket.Title, text}, "\n")
			if !containsFold(haystack, query) {
				continue
			}
			ticket.MatchingSnippets = snippets(text, query, 3)
		}
		if mentions != "" {
			ticket.MentionsTicketStem = true
		}
		out = append(out, ticket)
	}
	return out, nil
}

func TicketsStatus(root string, opts TicketStatusOptions) (*TicketInfo, error) {
	stem := strings.TrimSpace(opts.TicketStem)
	if stem == "" {
		return nil, fmt.Errorf("ticket_stem is required")
	}
	if !ticketStemRE.MatchString(stem) {
		return nil, fmt.Errorf("ticket_stem must be a ticket stem")
	}
	tickets, err := scanTickets(root, ticketScanOptions{
		IncludeDone:    opts.IncludeDone,
		IncludeDropped: opts.IncludeDropped,
		Resolve:        resolveModeFor(opts.Resolve),
	})
	if err != nil {
		return nil, err
	}
	for _, ticket := range tickets {
		if ticket.Stem == stem {
			copy := ticket
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("ticket not found: %s", stem)
}

// ticketResolveMode selects how much of the board one scan call is asking for.
// It is threaded per call site rather than inferred from which function is
// running, because the same entry points serve both discovery and resolution.
type ticketResolveMode int

const (
	// resolveOff is the zero value and today's behavior exactly: the walk is
	// filesystem-only and no scope is even constructed.
	resolveOff ticketResolveMode = iota
	// resolveGraph materializes hidden entries with Stem and Status derived
	// from the index path and only Parent parsed from the index body — the
	// bound loadTicketGraph needs and nothing more.
	resolveGraph
	// resolveFull materializes hidden entries with their whole body parsed, so
	// a text query can match them.
	resolveFull
)

func resolveModeFor(resolve bool) ticketResolveMode {
	if resolve {
		return resolveFull
	}
	return resolveOff
}

type ticketScanOptions struct {
	Statuses       []string
	IncludeDone    bool
	IncludeDropped bool
	Resolve        ticketResolveMode
}

func scanTickets(root string, opts ticketScanOptions) ([]TicketInfo, error) {
	tickets, _, err := scanTicketsWithBodies(root, opts)
	return tickets, err
}

// scanTicketsWithBodies is scanTickets plus the index bodies it read for the
// hidden entries, keyed by board-relative path. The map is only populated in
// resolveFull mode; TicketsFind is its sole consumer, so a hidden ticket's text
// query does not have to re-read the index.
//
// Under an active scope the enumeration is the UNION of the working tree and
// the index — never an index-only walk, which would drop uncommitted new
// tickets.
func scanTicketsWithBodies(root string, opts ticketScanOptions) ([]TicketInfo, map[string]string, error) {
	var scope *ticketScope
	if opts.Resolve != resolveOff {
		scope = newTicketScope(root)
	}

	ticketsRoot := filepath.Join(root, "ai-docs", "tickets")
	boardOnDisk := false
	if info, err := os.Stat(ticketsRoot); err == nil {
		if !info.IsDir() {
			return nil, nil, fmt.Errorf("tickets path is not a directory: %s", ticketsRoot)
		}
		boardOnDisk = true
	} else if scope == nil {
		return nil, nil, fmt.Errorf("tickets directory not found: %w", err)
	}
	// With a scope active, an absent board directory is not an error: git does
	// not track empty directories, so a fully-excluded status directory — and
	// in the limit ai-docs/tickets/ itself — vanishes from disk while the index
	// still holds every ticket. The walk then contributes nothing and the index
	// supplies the rest.

	statuses := ticketStatuses(opts)
	out := []TicketInfo{}
	if boardOnDisk {
		for _, status := range statuses {
			statusDir := filepath.Join(ticketsRoot, status)
			if !isDir(statusDir) {
				continue
			}
			for _, entry := range sortedEntries(statusDir) {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
					continue
				}
				path := filepath.Join(statusDir, entry.Name())
				info, err := readTicket(root, path, status)
				if err != nil {
					return nil, nil, err
				}
				out = append(out, info)
			}
		}
	}

	var bodies map[string]string
	if scope != nil {
		var err error
		out, bodies, err = appendHiddenTickets(scope, out, statuses, opts.Resolve)
		if err != nil {
			// The gate already confirmed a scope is active, so this failure
			// means the board is known-partial. Propagating it makes
			// loadTicketGraph return no advisories (TicketVerify swallows the
			// error) rather than emit false FIX: advisories over half a board.
			return nil, nil, err
		}
	}

	// Appended before the sort so loadTicketGraph's first-wins byStem rule,
	// which relies on ticketStatusRank ordering, keeps working unchanged.
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return ticketStatusRank(out[i].Status) < ticketStatusRank(out[j].Status)
		}
		return out[i].Stem < out[j].Stem
	})
	return out, bodies, nil
}

// appendHiddenTickets adds the index entries under the requested statuses that
// no working-tree file produced. Hidden-ness is defined as "in the index and
// not on disk", derived from the two sets already in hand rather than from
// `git ls-files -v`'s S tag, so a manually applied --skip-worktree on a file
// that is still present cannot change the answer.
func appendHiddenTickets(scope *ticketScope, onDisk []TicketInfo, statuses []string, mode ticketResolveMode) ([]TicketInfo, map[string]string, error) {
	wanted := map[string]bool{}
	for _, status := range statuses {
		wanted[status] = true
	}
	seen := make(map[string]bool, len(onDisk))
	for _, ticket := range onDisk {
		seen[ticket.Path] = true
	}

	paths, err := scope.indexPaths()
	if err != nil {
		return nil, nil, err
	}
	type hiddenEntry struct {
		rel    string
		status string
		stem   string
	}
	var hidden []hiddenEntry
	var relPaths []string
	for _, rel := range paths {
		status, stem, ok := ticketIndexPathParts(rel)
		if !ok || !wanted[status] || seen[rel] {
			continue
		}
		hidden = append(hidden, hiddenEntry{rel: rel, status: status, stem: stem})
		relPaths = append(relPaths, rel)
	}
	if len(hidden) == 0 {
		return onDisk, nil, nil
	}

	bodies, err := scope.bodies(relPaths)
	if err != nil {
		return nil, nil, err
	}
	// Key every hidden path, including one whose blob cat-file reported as
	// missing: TicketsFind falls back to os.ReadFile on an absent key, and for a
	// hidden path that read can only fail the whole call.
	resolved := make(map[string]string, len(hidden))
	for _, entry := range hidden {
		resolved[entry.rel] = bodies[entry.rel]
	}
	for _, entry := range hidden {
		body := resolved[entry.rel]
		if mode == resolveGraph {
			// Graph mode reads content for Parent alone; Stem and Status come
			// from the path. That is the whole-board bound loadTicketGraph
			// needs, and it keeps the commit path off full ticket parsing for
			// every hidden .done/.dropped entry.
			parent, _ := frontmatterFromText(body)["parent"].(string)
			onDisk = append(onDisk, TicketInfo{
				Stem:   entry.stem,
				Path:   entry.rel,
				Status: entry.status,
				Parent: parent,
				Hidden: true,
			})
			continue
		}
		info := readTicketFromBytes(entry.rel, entry.status, body)
		info.Hidden = true
		onDisk = append(onDisk, info)
	}
	if mode == resolveGraph {
		return onDisk, nil, nil
	}
	return onDisk, resolved, nil
}

func ticketStatuses(opts ticketScanOptions) []string {
	return EffectiveTicketStatuses(opts.Statuses, opts.IncludeDone, opts.IncludeDropped)
}

// EffectiveTicketStatuses is the single implementation of "which status
// directories does this request actually cover": the default open set when no
// status is named, plus the archive gating that drops an explicitly requested
// .done/.dropped unless the matching include flag is set.
//
// It is exported because a second caller outside wsdoc needs the same answer:
// the MCP layer's hidden-ticket annotation must count exactly the statuses the
// accompanying listing covered, or a listing that is empty because of the
// caller's own include flags gets blamed on the sparse-checkout scope. Sharing
// the rule keeps the two from drifting if a further archive status is ever
// added.
func EffectiveTicketStatuses(statuses []string, includeDone, includeDropped bool) []string {
	if len(statuses) == 0 {
		out := []string{"ready", "todo", "idea"}
		if includeDone {
			out = append(out, ".done")
		}
		if includeDropped {
			out = append(out, ".dropped")
		}
		return out
	}
	seen := map[string]bool{}
	out := []string{}
	for _, status := range statuses {
		normalized := normalizeTicketStatus(status)
		if normalized == "" || seen[normalized] {
			continue
		}
		if normalized == ".done" && !includeDone {
			continue
		}
		if normalized == ".dropped" && !includeDropped {
			continue
		}
		out = append(out, normalized)
		seen[normalized] = true
	}
	return out
}

func normalizeTicketStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "idea", "todo", "ready", "wip":
		return strings.TrimSpace(status)
	case "done", ".done":
		return ".done"
	case "dropped", ".dropped":
		return ".dropped"
	default:
		return ""
	}
}

func ticketStatusRank(status string) int {
	switch status {
	case "ready":
		return 0
	case "todo":
		return 1
	case "idea":
		return 2
	case "wip":
		return 3
	case ".done":
		return 4
	case ".dropped":
		return 5
	default:
		return 9
	}
}

func readTicket(root, path, status string) (TicketInfo, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return TicketInfo{}, err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return TicketInfo{}, err
	}
	return readTicketFromBytes(filepath.ToSlash(rel), status, string(raw)), nil
}

// readTicketFromBytes is the parse half of readTicket, taking the already-read
// text so the disk path and the index-blob path share one implementation. It
// also removes readTicket's former double read of every ticket file (once for
// the body, once inside frontmatter(path)).
func readTicketFromBytes(relPath, status, text string) TicketInfo {
	fm := frontmatterFromText(text)
	phases, resultPresent := ticketPhases(text)
	info := TicketInfo{
		Stem:          ticketStemFromRelPath(relPath),
		Path:          relPath,
		Status:        status,
		ResultPresent: resultPresent,
		Phases:        phases,
	}
	info.Title, _ = fm["title"].(string)
	info.Parent, _ = fm["parent"].(string)
	info.Related = relatedEntries(fm["related"])
	info.Specs = scalarList(fm["spec"])
	info.SpecRemoves = scalarList(fm["spec-remove"])
	info.Plans = scalarList(fm["plans"])
	info.Skeletons = scalarList(fm["skeletons"])
	info.Completed, _ = fm["completed"].(string)
	for _, phase := range phases {
		if !phase.ResultPresent {
			info.UnresolvedPhases = append(info.UnresolvedPhases, phase.Heading)
		}
	}
	return info
}

// ticketStemFromRelPath takes the basename of a forward-slash board-relative
// path; filepath.Base is avoided so an index path parses identically on
// Windows, where the separator differs from the one git reports.
func ticketStemFromRelPath(relPath string) string {
	if idx := strings.LastIndex(relPath, "/"); idx >= 0 {
		relPath = relPath[idx+1:]
	}
	return strings.TrimSuffix(relPath, ".md")
}

func ticketPhases(text string) ([]TicketPhase, bool) {
	lines := strings.Split(text, "\n")
	phases := []TicketPhase{}
	current := -1
	anyResult := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### Phase ") {
			phases = append(phases, TicketPhase{Heading: strings.TrimPrefix(trimmed, "### ")})
			current = len(phases) - 1
			continue
		}
		if strings.HasPrefix(trimmed, "### Result") {
			anyResult = true
			if current >= 0 {
				phases[current].ResultPresent = true
			}
		}
	}
	return phases, anyResult
}

// relatedEntries normalises every frontmatter shape the hand-rolled parser
// (frontmatter.go) can produce for `related:` into the stem -> note map
// TicketInfo advertises. The nested `key: note` form already arrives as
// map[string]string, but the equally legal list form (`- <stem>` items)
// arrives as []string and used to be dropped on the floor by a bare type
// assertion, so a list-form related: silently resolved to nil. List items also
// never pass through cleanScalar in the parser, so a trailing " # comment"
// stays glued to the item and is stripped here. Normalising is what lets the
// cross-reference integrity checks (tickets_graph.go) see a frontmatter shape
// they would otherwise skip without saying so — the ticket forbids advertising
// a floor the check does not cover.
func relatedEntries(value any) map[string]string {
	switch typed := value.(type) {
	case map[string]string:
		return typed
	case []string:
		out := map[string]string{}
		for _, item := range typed {
			item = cleanScalar(item)
			if item == "" {
				continue
			}
			stem, note := item, ""
			if idx := strings.Index(item, ":"); idx >= 0 {
				stem = strings.TrimSpace(item[:idx])
				note = strings.TrimSpace(item[idx+1:])
			}
			if stem == "" {
				continue
			}
			out[stem] = note
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		stem := cleanScalar(typed)
		if stem == "" {
			return nil
		}
		return map[string]string{stem: ""}
	default:
		return nil
	}
}

func scalarList(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []string:
		out := []string{}
		for _, item := range typed {
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	default:
		return nil
	}
}

func containsFold(text, query string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(query))
}

func snippets(text, query string, limit int) []string {
	if query == "" || limit <= 0 {
		return nil
	}
	lowerQuery := strings.ToLower(query)
	out := []string{}
	for _, line := range strings.Split(text, "\n") {
		if containsFold(line, lowerQuery) {
			out = append(out, strings.TrimSpace(line))
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
