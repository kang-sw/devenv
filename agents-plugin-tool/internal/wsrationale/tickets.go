package wsrationale

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ticketStatusDirs enumerates every status directory a recorded decision may
// live under, including the archive statuses. The status field drops the dot.
var ticketStatusDirs = []struct {
	dir    string
	status string
}{
	{"idea", "idea"},
	{"todo", "todo"},
	{"ready", "ready"},
	{"wip", "wip"},
	{".done", "done"},
	{".dropped", "dropped"},
}

// ticketSections are the level-2 rationale sections, addressed by exact heading.
var ticketSections = []struct {
	heading string
	prefix  string
}{
	{"## Decisions", "## "},
	{"## Cross-Child Decisions", "## "},
	{"## Rejected Alternatives", "## "},
	{"## Open Questions", "## "},
	{"## Resolution", "## "},
	{"### Confirmed Decisions", "### "},
}

// resultHeadingRE matches a per-phase Result heading, `### Result (<hash>) - <date>`.
var resultHeadingRE = regexp.MustCompile(`^### Result\b.*$`)

// dateInTextRE finds an ISO date inside a heading line.
var dateInTextRE = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

// backtickTokenRE captures backticked tokens; pathRefRE and extRE then decide
// which are file paths.
var backtickTokenRE = regexp.MustCompile("`([^`]+)`")

// extRE matches a path that ends in a file extension.
var extRE = regexp.MustCompile(`\.[A-Za-z0-9]+$`)

type rawTicket struct {
	stem   string
	status string
	title  string
	body   string
	paths  []string
}

// scanTickets walks every status directory under ai-docs/tickets and returns
// one rawTicket per file. A missing board directory yields no tickets rather
// than an error: a repository may simply have no tickets.
func scanTickets(root string) ([]rawTicket, error) {
	base := filepath.Join(root, "ai-docs", "tickets")
	var tickets []rawTicket
	for _, sd := range ticketStatusDirs {
		dir := filepath.Join(base, sd.dir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // absent status directory
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
				continue
			}
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, err
			}
			body := string(raw)
			tickets = append(tickets, rawTicket{
				stem:   strings.TrimSuffix(entry.Name(), ".md"),
				status: sd.status,
				title:  ticketTitle(body),
				body:   body,
				paths:  ticketBodyPaths(body),
			})
		}
	}
	return tickets, nil
}

// ticketTitle reads the frontmatter `title:` scalar, falling back to the empty
// string. A minimal parser suffices: only the top-level title scalar is needed.
func ticketTitle(body string) string {
	if !strings.HasPrefix(body, "---") {
		return ""
	}
	lines := strings.Split(body, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			break
		}
		line := lines[i]
		if strings.HasPrefix(line, "title:") {
			return unquote(strings.TrimSpace(strings.TrimPrefix(line, "title:")))
		}
	}
	return ""
}

func unquote(v string) string {
	if len(v) >= 2 {
		if (v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'') {
			return v[1 : len(v)-1]
		}
	}
	return v
}

// ticketBodyPaths extracts the backticked tokens that look like file paths
// (contain `/` and end in a file extension), plus `path#L` references, with the
// `#L...` suffix stripped to the bare path for glob matching. Order preserved,
// deduped.
func ticketBodyPaths(body string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	for _, m := range backtickTokenRE.FindAllStringSubmatch(body, -1) {
		token := strings.TrimSpace(m[1])
		p := token
		if idx := strings.Index(p, "#L"); idx >= 0 {
			p = p[:idx]
		}
		if !strings.Contains(p, "/") {
			continue
		}
		if !extRE.MatchString(p) {
			continue
		}
		add(p)
	}
	return out
}

// stemDate converts a stem's leading YYMMDD to 20YY-MM-DD.
func stemDate(stem string) string {
	if len(stem) < 6 {
		return ""
	}
	d := stem[:6]
	for i := 0; i < 6; i++ {
		if d[i] < '0' || d[i] > '9' {
			return ""
		}
	}
	return "20" + d[0:2] + "-" + d[2:4] + "-" + d[4:6]
}

// ticketRecords turns one ticket into its rationale records, one per bullet or
// paragraph across every recognized section.
func ticketRecords(t rawTicket) []Record {
	fallbackDate := stemDate(t.stem)
	var records []Record

	emit := func(secs []section) {
		for _, sec := range secs {
			sectionText := strings.TrimSpace(strings.TrimLeft(sec.heading, "#"))
			date := fallbackDate
			if d := dateInTextRE.FindString(sec.heading); d != "" {
				date = d
			}
			for _, text := range splitRecords(sec.lines) {
				records = append(records, Record{
					Kind:    "ticket",
					Stem:    t.stem,
					Status:  t.status,
					Section: sectionText,
					Date:    date,
					Text:    text,
					Paths:   append([]string(nil), t.paths...),
					Stems:   []string{t.stem},
				})
			}
		}
	}

	for _, s := range ticketSections {
		emit(collectSections(t.body, exactOrPrefixHeading(s.heading), s.prefix))
	}
	// Every per-phase `### Result (<hash>) - <date>` heading is its own section.
	emit(collectSections(t.body, func(trimmed string) bool {
		return resultHeadingRE.MatchString(trimmed)
	}, "### "))
	return records
}
