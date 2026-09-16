package wsrationale

import (
	"fmt"
	"strings"
)

const (
	compactQuoteCut = 300
	maxThreadPaths  = 5
)

func shortHash(hash string) string {
	if len(hash) >= 8 {
		return hash[:8]
	}
	return hash
}

func cut(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}

// FormatText renders the compact default output. The `site:` line appears only
// in site mode; the `omitted:` line is always last and always carries the
// scanned-commit count, so it is never empty.
func FormatText(res Result) string {
	var b strings.Builder
	for _, th := range res.Threads {
		writeThread(&b, th)
	}
	if res.isSite && res.Site != nil {
		writeSiteLine(&b, res.Site)
	}
	writeOmitted(&b, res)
	return b.String()
}

func writeThread(b *strings.Builder, th Thread) {
	header := "(no ticket)"
	if th.Stem != "" {
		header = th.Stem
		if th.Status != "" {
			header += " (" + th.Status + ")"
		}
	}
	fmt.Fprintf(b, "thread: %s — %d records\n", header, len(th.Records))
	for _, rec := range th.Records {
		quote := cut(rec.Text, compactQuoteCut)
		if rec.Kind == "ticket" {
			fmt.Fprintf(b, "  %s ticket %s — %s\n", rec.Date, rec.Section, quote)
		} else {
			fmt.Fprintf(b, "  %s %s — %s\n", rec.Date, shortHash(rec.Hash), quote)
		}
	}
	if paths := threadPaths(th); len(paths) > 0 {
		b.WriteString("  paths: ")
		b.WriteString(formatThreadPaths(paths))
		b.WriteString("\n")
	}
}

// threadPaths is the union of distinct touched paths across a thread's records,
// in first-seen order.
func threadPaths(th Thread) []string {
	seen := map[string]bool{}
	var out []string
	for _, rec := range th.Records {
		for _, p := range rec.Paths {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	return out
}

func formatThreadPaths(paths []string) string {
	if len(paths) <= maxThreadPaths {
		return strings.Join(paths, " ")
	}
	shown := strings.Join(paths[:maxThreadPaths], " ")
	return fmt.Sprintf("%s +%d more", shown, len(paths)-maxThreadPaths)
}

func writeSiteLine(b *strings.Builder, site *SiteSummary) {
	intro := "unknown"
	if site.Introduced != nil {
		intro = fmt.Sprintf("%s %s", shortHash(site.Introduced.Hash), site.Introduced.Date)
	}
	last := "unknown"
	if site.LastChanged != nil {
		last = fmt.Sprintf("%s %s", shortHash(site.LastChanged.Hash), site.LastChanged.Date)
	}
	fmt.Fprintf(b, "site: introduced %s; last changed %s; %d changes\n", intro, last, site.Changes)
}

func writeOmitted(b *strings.Builder, res Result) {
	parts := []string{fmt.Sprintf("scanned %d commits", res.ScannedCommits)}
	parts = append(parts, res.Omitted...)
	fmt.Fprintf(b, "omitted: %s\n", strings.Join(parts, "; "))
}

// --- JSON output ---

type jsonResult struct {
	Threads        []jsonThread `json:"threads"`
	Site           *jsonSite    `json:"site"`
	ScannedCommits int          `json:"scanned_commits"`
	Truncated      bool         `json:"truncated"`
	Omitted        []string     `json:"omitted"`
}

type jsonThread struct {
	Stem    *string      `json:"stem"`
	Status  *string      `json:"status"`
	Title   *string      `json:"title"`
	Records []jsonRecord `json:"records"`
}

type jsonRecord struct {
	Kind    string   `json:"kind"`
	Pointer string   `json:"pointer"`
	Date    string   `json:"date"`
	Text    string   `json:"text"`
	Subject *string  `json:"subject"`
	Paths   []string `json:"paths"`
	Stems   []string `json:"stems"`
	Score   float64  `json:"score"`
}

type jsonSite struct {
	Introduced  *ChainPoint `json:"introduced"`
	LastChanged *ChainPoint `json:"last_changed"`
	Changes     int         `json:"changes"`
	Truncated   bool        `json:"truncated"`
}

// JSON returns the structured output value for format=json.
func JSON(res Result) any {
	out := jsonResult{
		ScannedCommits: res.ScannedCommits,
		Truncated:      res.Truncated,
		Omitted:        res.Omitted,
	}
	if out.Omitted == nil {
		out.Omitted = []string{}
	}
	for _, th := range res.Threads {
		jt := jsonThread{Records: []jsonRecord{}}
		if th.Stem != "" {
			jt.Stem = strPtr(th.Stem)
		}
		if th.Status != "" {
			jt.Status = strPtr(th.Status)
		}
		if th.Title != "" {
			jt.Title = strPtr(th.Title)
		}
		for _, rec := range th.Records {
			jr := jsonRecord{
				Kind:    rec.Kind,
				Pointer: rec.Pointer(),
				Date:    rec.Date,
				Text:    rec.Text,
				Paths:   rec.Paths,
				Stems:   rec.Stems,
				Score:   rec.Score,
			}
			if rec.Kind == "commit" && rec.Subject != "" {
				jr.Subject = strPtr(rec.Subject)
			}
			if jr.Paths == nil {
				jr.Paths = []string{}
			}
			if jr.Stems == nil {
				jr.Stems = []string{}
			}
			jt.Records = append(jt.Records, jr)
		}
		out.Threads = append(out.Threads, jt)
	}
	if out.Threads == nil {
		out.Threads = []jsonThread{}
	}
	if res.Site != nil {
		out.Site = &jsonSite{
			Introduced:  res.Site.Introduced,
			LastChanged: res.Site.LastChanged,
			Changes:     res.Site.Changes,
			Truncated:   res.Site.Truncated,
		}
	}
	return out
}

func strPtr(s string) *string { return &s }
