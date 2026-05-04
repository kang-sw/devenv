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
}

type TicketStatusOptions struct {
	TicketStem     string
	IncludeDone    bool
	IncludeDropped bool
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
	tickets, err := scanTickets(root, ticketScanOptions{
		Statuses:       opts.Statuses,
		IncludeDone:    opts.IncludeDone,
		IncludeDropped: opts.IncludeDropped,
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
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ticket.Path)))
		if err != nil {
			return nil, err
		}
		text := string(raw)
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

type ticketScanOptions struct {
	Statuses       []string
	IncludeDone    bool
	IncludeDropped bool
}

func scanTickets(root string, opts ticketScanOptions) ([]TicketInfo, error) {
	ticketsRoot := filepath.Join(root, "ai-docs", "tickets")
	info, err := os.Stat(ticketsRoot)
	if err != nil {
		return nil, fmt.Errorf("tickets directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("tickets path is not a directory: %s", ticketsRoot)
	}

	out := []TicketInfo{}
	for _, status := range ticketStatuses(opts) {
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
				return nil, err
			}
			out = append(out, info)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return ticketStatusRank(out[i].Status) < ticketStatusRank(out[j].Status)
		}
		return out[i].Stem < out[j].Stem
	})
	return out, nil
}

func ticketStatuses(opts ticketScanOptions) []string {
	if len(opts.Statuses) == 0 {
		statuses := []string{"idea", "todo", "wip"}
		if opts.IncludeDone {
			statuses = append(statuses, ".done")
		}
		if opts.IncludeDropped {
			statuses = append(statuses, ".dropped")
		}
		return statuses
	}
	seen := map[string]bool{}
	out := []string{}
	for _, status := range opts.Statuses {
		normalized := normalizeTicketStatus(status)
		if normalized == "" || seen[normalized] {
			continue
		}
		if normalized == ".done" && !opts.IncludeDone {
			continue
		}
		if normalized == ".dropped" && !opts.IncludeDropped {
			continue
		}
		out = append(out, normalized)
		seen[normalized] = true
	}
	return out
}

func normalizeTicketStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "idea", "todo", "wip":
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
	case "idea":
		return 0
	case "todo":
		return 1
	case "wip":
		return 2
	case ".done":
		return 3
	case ".dropped":
		return 4
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
	fm := frontmatter(path)
	phases, resultPresent := ticketPhases(string(raw))
	info := TicketInfo{
		Stem:          strings.TrimSuffix(filepath.Base(path), ".md"),
		Path:          filepath.ToSlash(rel),
		Status:        status,
		ResultPresent: resultPresent,
		Phases:        phases,
	}
	info.Title, _ = fm["title"].(string)
	info.Parent, _ = fm["parent"].(string)
	info.Related, _ = fm["related"].(map[string]string)
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
	return info, nil
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
