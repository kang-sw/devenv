// Package wsrationale implements the read-only rationale.query MCP tool: a
// lexical search over recorded rationale — commit `## AI Context` and
// `## Ticket Updates` bullets and ticket decision sections — addressed by path
// globs, a code site (git log -L), a pickaxe string (git log -S), or a text
// query. It writes nothing: every source it reads (commit bodies, ticket files)
// is ground truth that does not rot, so v1 has no cache.
package wsrationale

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

const (
	defaultLimit = 30
	maxLimit     = 100
	noTicket     = "(no ticket)"
)

// Options is the parsed rationale.query input. All fields are optional unless a
// mode requires one.
type Options struct {
	Query       string
	Paths       []string
	Site        string
	Occurrence  int
	Pickaxe     string
	Since       string
	Until       string
	Stems       []string
	ExcludeStem string
	Kinds       []string
	Order       string
	Limit       int
}

// Record is one bullet or paragraph of rationale with a pointer back to its
// source.
type Record struct {
	Kind    string   `json:"kind"`
	Hash    string   `json:"-"`
	Stem    string   `json:"-"`
	Section string   `json:"-"`
	Status  string   `json:"-"`
	Date    string   `json:"date"`
	Subject string   `json:"subject,omitempty"`
	Text    string   `json:"text"`
	Paths   []string `json:"paths,omitempty"`
	Stems   []string `json:"stems,omitempty"`
	Score   float64  `json:"score"`
}

// Pointer returns the record's addressable pointer: the full hash for a commit,
// `<stem>#<section>` for a ticket.
func (r Record) Pointer() string {
	if r.Kind == "commit" {
		return r.Hash
	}
	return r.Stem + "#" + r.Section
}

// docTokens returns the ranking document: the record text plus the commit
// subject or ticket title (carried on Subject for both — see buildRecords).
func (r Record) docTokens() []string {
	return tokenize(r.Text + " " + r.Subject)
}

// threadKeys returns the thread stems a record belongs to. A commit with no
// stem belongs to the (no ticket) thread.
func (r Record) threadKeys() []string {
	if r.Kind == "ticket" {
		return []string{r.Stem}
	}
	if len(r.Stems) == 0 {
		return []string{noTicket}
	}
	return r.Stems
}

// Thread is one ticket stem plus every record grouped under it.
type Thread struct {
	Stem    string   `json:"stem"`
	Status  string   `json:"status"`
	Title   string   `json:"title"`
	Records []Record `json:"records"`
}

// SiteSummary is the git log -L chain summary returned in site mode.
type SiteSummary struct {
	Introduced  *ChainPoint `json:"introduced"`
	LastChanged *ChainPoint `json:"last_changed"`
	Changes     int         `json:"changes"`
	Truncated   bool        `json:"truncated"`
}

// ChainPoint is one end of the site chain.
type ChainPoint struct {
	Hash string `json:"hash"`
	Date string `json:"date"`
}

// Result is the whole query result.
type Result struct {
	Threads        []Thread     `json:"threads"`
	Site           *SiteSummary `json:"site"`
	ScannedCommits int          `json:"scanned_commits"`
	Truncated      bool         `json:"truncated"`
	Omitted        []string     `json:"omitted"`

	// isSite drives the compact `site:` line and is not serialized.
	isSite bool
}

// Query executes a rationale search over root. runner defaults to real git.
func Query(ctx context.Context, runner wsgit.Runner, root string, opts Options) (Result, error) {
	if runner == nil {
		runner = wsgit.ExecRunner{}
	}
	if err := validateOptions(opts); err != nil {
		return Result{}, err
	}

	since, until, err := parseDateBounds(opts.Since, opts.Until)
	if err != nil {
		return Result{}, err
	}

	switch {
	case opts.Site != "":
		return querySite(ctx, runner, root, opts, since, until)
	case opts.Pickaxe != "":
		return queryPickaxe(ctx, runner, root, opts, since, until)
	default:
		return queryScan(ctx, runner, root, opts, since, until)
	}
}

// validateOptions enforces the mutually exclusive addressing modes and glob
// syntax up front, so a malformed request is an error rather than an empty
// result.
func validateOptions(opts Options) error {
	modes := 0
	if len(opts.Paths) > 0 {
		modes++
	}
	if opts.Site != "" {
		modes++
	}
	if opts.Pickaxe != "" {
		modes++
	}
	if modes > 1 {
		return fmt.Errorf("paths, site, and pickaxe are mutually exclusive; pass at most one")
	}
	for _, p := range opts.Paths {
		if err := validateGlob(p); err != nil {
			return err
		}
	}
	if opts.Site != "" && strings.TrimSpace(opts.Site) == "" {
		return fmt.Errorf("site may not be blank")
	}
	if opts.Pickaxe != "" && strings.TrimSpace(opts.Pickaxe) == "" {
		return fmt.Errorf("pickaxe may not be blank")
	}
	return nil
}

// queryScan handles default, path, and text-query modes: scan all of HEAD plus
// every ticket, filter, rank, and group.
func queryScan(ctx context.Context, runner wsgit.Runner, root string, opts Options, since, until string) (Result, error) {
	commits, truncated, err := scanCommits(ctx, runner, root, "")
	if err != nil {
		return Result{}, err
	}
	tickets, err := scanTickets(root)
	if err != nil {
		return Result{}, err
	}
	records := buildRecords(commits, tickets)
	res := assemble(records, opts, since, until)
	res.ScannedCommits = len(commits)
	res.Truncated = truncated
	if truncated {
		res.Omitted = append(res.Omitted, fmt.Sprintf("truncated at %d", scanCap))
	}
	return res, nil
}

// queryPickaxe handles pickaxe mode: a `git log -S` scan, commit records only.
func queryPickaxe(ctx context.Context, runner wsgit.Runner, root string, opts Options, since, until string) (Result, error) {
	commits, truncated, err := scanCommits(ctx, runner, root, opts.Pickaxe)
	if err != nil {
		return Result{}, err
	}
	records := buildRecords(commits, nil)
	res := assemble(records, opts, since, until)
	res.ScannedCommits = len(commits)
	res.Truncated = truncated
	if truncated {
		res.Omitted = append(res.Omitted, fmt.Sprintf("truncated at %d", scanCap))
	}
	return res, nil
}

// querySite handles site mode: resolve the address, walk the `git log -L`
// chain, build commit records only, and attach the chain summary.
func querySite(ctx context.Context, runner wsgit.Runner, root string, opts Options, since, until string) (Result, error) {
	site, err := parseSite(ctx, runner, root, opts.Site, opts.Occurrence)
	if err != nil {
		return Result{}, err
	}
	commits, chainTruncated, err := scanSiteChain(ctx, runner, root, site)
	if err != nil {
		return Result{}, err
	}
	records := buildRecords(commits, nil)
	res := assemble(records, opts, since, until)
	res.ScannedCommits = len(commits)
	res.isSite = true

	summary := &SiteSummary{Changes: len(commits), Truncated: chainTruncated}
	if len(commits) > 0 {
		newest := commits[0]
		summary.LastChanged = &ChainPoint{Hash: newest.hash, Date: newest.date}
		if !chainTruncated {
			oldest := commits[len(commits)-1]
			summary.Introduced = &ChainPoint{Hash: oldest.hash, Date: oldest.date}
		}
	}
	res.Site = summary

	if site.regexForm && opts.Occurrence < 1 && site.matchCount > 1 {
		res.Omitted = append(res.Omitted, fmt.Sprintf("site matched %d locations; used occurrence 1", site.matchCount))
	}
	if chainTruncated {
		res.Omitted = append(res.Omitted, fmt.Sprintf("site chain truncated at %d", siteChainCap))
	}
	res.Omitted = append(res.Omitted, "renames not followed")
	return res, nil
}

// buildRecords turns raw commits and tickets into the flat record list, wiring
// per-stem commit-path unions into ticket records so a ticket record matches a
// path filter through a commit in its thread. It also copies the ticket title
// into Record.Subject so the ranking document is uniform.
func buildRecords(commits []rawCommit, tickets []rawTicket) []Record {
	var commitRecs []Record
	stemCommitPaths := map[string]map[string]bool{}
	for _, c := range commits {
		recs := commitRecords(c)
		commitRecs = append(commitRecs, recs...)
		for _, stem := range extractStems(c.body) {
			if stemCommitPaths[stem] == nil {
				stemCommitPaths[stem] = map[string]bool{}
			}
			for _, p := range c.paths {
				stemCommitPaths[stem][p] = true
			}
		}
	}

	var ticketRecs []Record
	for _, t := range tickets {
		recs := ticketRecords(t)
		extra := stemCommitPaths[t.stem]
		for i := range recs {
			recs[i].Subject = t.title // ranking document carries the title
			if len(extra) > 0 {
				recs[i].Paths = mergePaths(recs[i].Paths, extra)
			}
		}
		ticketRecs = append(ticketRecs, recs...)
	}

	return append(commitRecs, ticketRecs...)
}

func mergePaths(existing []string, extra map[string]bool) []string {
	seen := map[string]bool{}
	out := append([]string(nil), existing...)
	for _, p := range existing {
		seen[p] = true
	}
	keys := make([]string, 0, len(extra))
	for p := range extra {
		if !seen[p] {
			keys = append(keys, p)
		}
	}
	sort.Strings(keys)
	return append(out, keys...)
}

// assemble applies every post-scan filter, ranks, caps at limit, and groups the
// surviving records into ordered threads. It sets Omitted's records-past-limit
// note and leaves scan-count/truncation to the caller.
func assemble(records []Record, opts Options, since, until string) Result {
	stats := newBM25(records)
	queryTokens := tokenize(opts.Query)

	kinds := kindSet(opts.Kinds)
	stemFilter := stringSet(opts.Stems)

	// Filter.
	candidates := records[:0:0]
	for _, rec := range records {
		if !kinds[rec.Kind] {
			continue
		}
		if !dateInBounds(rec.Date, since, until) {
			continue
		}
		if len(stemFilter) > 0 && !recordInStems(rec, stemFilter) {
			continue
		}
		if opts.ExcludeStem != "" && recordExcluded(rec, opts.ExcludeStem) {
			continue
		}
		if len(opts.Paths) > 0 && !matchAnyGlob(opts.Paths, rec.Paths) {
			continue
		}
		if len(queryTokens) > 0 {
			rec.Score = stats.score(queryTokens, rec)
		}
		candidates = append(candidates, rec)
	}

	order := effectiveOrder(opts.Order, opts.Query)
	sortRecords(candidates, order)

	limit := effectiveLimit(opts.Limit)
	matched := len(candidates)
	if matched > limit {
		candidates = candidates[:limit]
	}

	res := Result{Threads: groupThreads(candidates, order)}
	if matched > limit {
		res.Omitted = append(res.Omitted, fmt.Sprintf("%d records past limit", matched-limit))
	}
	return res
}

func effectiveOrder(order, query string) string {
	switch order {
	case "relevance", "time":
		return order
	default:
		if strings.TrimSpace(query) != "" {
			return "relevance"
		}
		return "time"
	}
}

func effectiveLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

// sortRecords orders records by relevance (score desc, then date desc, then
// hash) or time (date desc, then hash). Pointer breaks final ties for stability.
func sortRecords(records []Record, order string) {
	sort.SliceStable(records, func(i, j int) bool {
		a, b := records[i], records[j]
		if order == "relevance" && a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Date != b.Date {
			return a.Date > b.Date
		}
		return a.Pointer() < b.Pointer()
	})
}

// groupThreads assembles the capped record list into threads. A record appears
// under each of its thread keys but was counted once against the limit. Threads
// are ordered by their best record; records within a thread are date descending.
func groupThreads(records []Record, order string) []Thread {
	index := map[string]*Thread{}
	var keysInOrder []string
	best := map[string]Record{}
	for _, rec := range records {
		for _, key := range rec.threadKeys() {
			th, ok := index[key]
			if !ok {
				th = &Thread{}
				if key != noTicket {
					th.Stem = key
				}
				index[key] = th
				keysInOrder = append(keysInOrder, key)
				best[key] = rec
			}
			th.Records = append(th.Records, rec)
			if recordRanksBefore(rec, best[key], order) {
				best[key] = rec
			}
			// A ticket record carries the authoritative status/title for its
			// thread.
			if rec.Kind == "ticket" && key == rec.Stem {
				th.Status = rec.Status
				th.Title = rec.Subject
			}
		}
	}

	threads := make([]Thread, 0, len(keysInOrder))
	for _, key := range keysInOrder {
		th := index[key]
		sortRecords(th.Records, "time") // within a thread, always date descending
		threads = append(threads, *th)
	}
	sort.SliceStable(threads, func(i, j int) bool {
		return recordRanksBefore(best[threadKey(threads[i])], best[threadKey(threads[j])], order)
	})
	return threads
}

func threadKey(t Thread) string {
	if t.Stem == "" {
		return noTicket
	}
	return t.Stem
}

// recordRanksBefore reports whether a should sort before b under the order.
func recordRanksBefore(a, b Record, order string) bool {
	if order == "relevance" && a.Score != b.Score {
		return a.Score > b.Score
	}
	if a.Date != b.Date {
		return a.Date > b.Date
	}
	return a.Pointer() < b.Pointer()
}

func recordInStems(rec Record, stems map[string]bool) bool {
	for _, key := range rec.threadKeys() {
		if stems[key] {
			return true
		}
	}
	return false
}

// recordExcluded drops the excluded ticket's own records and commits that name
// only that stem.
func recordExcluded(rec Record, exclude string) bool {
	if rec.Kind == "ticket" {
		return rec.Stem == exclude
	}
	return len(rec.Stems) == 1 && rec.Stems[0] == exclude
}

func kindSet(kinds []string) map[string]bool {
	out := map[string]bool{}
	for _, k := range kinds {
		switch k {
		case "commit", "ticket":
			out[k] = true
		}
	}
	if len(out) == 0 {
		return map[string]bool{"commit": true, "ticket": true}
	}
	return out
}

func stringSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out[v] = true
		}
	}
	return out
}
