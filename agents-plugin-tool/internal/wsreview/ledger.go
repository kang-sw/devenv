// Package wsreview implements the review-watermark ledger: a single tracked,
// append-only, line-oriented text file recording review verdicts per commit
// range so a caller can resolve "what has already been reviewed" by reading
// the last entry, never by graph-walking or by asking git which commit last
// touched the file.
package wsreview

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// Entry represents one parsed ledger line: a reviewed commit range and its
// verdict, plus an optional routed-ticket-stem reference.
//
// Verdict is one of: pass, concern, block, routed, bootstrap. The last two
// are not genuine review outcomes — routed is the append-only corrective
// follow-up for a malformed un-routed block entry, and bootstrap is the
// explicit-bootstrap surfacing token emitted by Bootstrap (see below).
type Entry struct {
	Base    string
	Head    string
	Verdict string
	Ref     string
}

const (
	VerdictPass      = "pass"
	VerdictConcern   = "concern"
	VerdictBlock     = "block"
	VerdictRouted    = "routed"
	VerdictBootstrap = "bootstrap"
)

// entryLineRE matches one well-formed ledger entry line:
//
//	<base>..<head>: <verdict>[ -> <ref>]
//
// Any line that does not match — including every #-prefixed comment/banner
// line and blank lines — is skipped by the parser. This is deliberate
// Phase-3 readiness: the eventual top-of-file banner and tail-anchor comment
// must never be mistaken for the latest entry.
var entryLineRE = regexp.MustCompile(`^([0-9a-f]{7,40})\.\.([0-9a-f]{7,40}): (pass|concern|block|routed|bootstrap)(?: -> (\S+))?\s*$`)

// LedgerPath resolves the tracked review-ledger file path under root,
// mirroring wsnote.RepoDir's join style.
func LedgerPath(root string) string {
	return filepath.Join(root, "ai-docs", ".review-ledger.md")
}

// ParseLatest scans content top to bottom and returns the last line that
// matches entryLineRE, keeping the running last match rather than stopping
// at the first. Lines that don't match — comments, banners, blank lines, or
// any other unrelated edit — are skipped. Returns (Entry{}, false) when no
// line in content matches (empty or comment-only ledger).
func ParseLatest(content string) (Entry, bool) {
	var (
		latest Entry
		found  bool
	)
	for _, line := range strings.Split(content, "\n") {
		m := entryLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		latest = Entry{Base: m[1], Head: m[2], Verdict: m[3], Ref: m[4]}
		found = true
	}
	return latest, found
}

// Read reads the ledger file at LedgerPath(root) and returns its latest
// entry via ParseLatest. A missing file returns (Entry{}, false, nil) — not
// an error — mirroring wsnote.Load's "no file yet" contract.
func Read(root string) (Entry, bool, error) {
	raw, err := os.ReadFile(LedgerPath(root))
	if os.IsNotExist(err) {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, fmt.Errorf("read review ledger %s: %w", LedgerPath(root), err)
	}
	entry, found := ParseLatest(string(raw))
	return entry, found, nil
}

// Append validates e and appends one formatted entry line to the ledger at
// LedgerPath(root), creating the parent ai-docs/ directory and the ledger
// file itself if absent. Append never mutates existing lines — this is
// structural, not just documented: the file is opened with
// O_APPEND|O_CREATE|O_WRONLY and only ever written to, never read-modified.
//
// Validation: Base and Head must be non-empty SHA-shaped strings, Verdict
// must be a known token, and Ref must be non-empty whenever Verdict is
// block — the ticket's explicit "the append surface requires a stem on a
// block entry" invariant. A concern entry MAY carry a Ref but is not
// required to: per the 2026-08-30 lead adjudication, the routed-stem
// requirement is block-only, since the release gate's forcing function
// blocks promotion on an unresolved *blocking* finding only.
func Append(root string, e Entry) error {
	if !shaLikeRE.MatchString(e.Base) {
		return fmt.Errorf("review ledger append: Base is not SHA-shaped: %q", e.Base)
	}
	if !shaLikeRE.MatchString(e.Head) {
		return fmt.Errorf("review ledger append: Head is not SHA-shaped: %q", e.Head)
	}
	switch e.Verdict {
	case VerdictPass, VerdictConcern, VerdictBlock, VerdictRouted, VerdictBootstrap:
	default:
		return fmt.Errorf("review ledger append: unknown verdict: %q", e.Verdict)
	}
	if e.Verdict == VerdictBlock && strings.TrimSpace(e.Ref) == "" {
		return fmt.Errorf("review ledger append: verdict %q requires a non-empty Ref (routed ticket stem)", VerdictBlock)
	}

	path := LedgerPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create review ledger dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open review ledger %s: %w", path, err)
	}
	defer f.Close()

	if _, err := f.WriteString(formatEntry(e)); err != nil {
		return fmt.Errorf("write review ledger %s: %w", path, err)
	}
	return nil
}

// shaLikeRE bounds Base/Head validation to the same SHA shape the parser
// accepts, so a validated Append always round-trips through ParseLatest.
var shaLikeRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func formatEntry(e Entry) string {
	if e.Ref != "" {
		return fmt.Sprintf("%s..%s: %s -> %s\n", e.Base, e.Head, e.Verdict, e.Ref)
	}
	return fmt.Sprintf("%s..%s: %s\n", e.Base, e.Head, e.Verdict)
}

// Bootstrap ensures the ledger has at least one parseable entry, explicitly
// surfacing that prior history is review-skipped rather than reviewed.
//
// If Read already finds an entry, Bootstrap is a no-op and returns that
// entry with created = false (idempotent — a second call is safe). If
// absent (file missing, or present but zero parseable entries — e.g.
// banner-only), Bootstrap resolves current HEAD via
// wsgit.ExecRunner{}.RunGit(ctx, root, "rev-parse", "HEAD") and appends an
// Entry{Base: head, Head: head, Verdict: "bootstrap"} line, returning it
// with created = true. The distinct bootstrap verdict token is itself the
// explicit surface: a future reader can render "prior history is
// review-skipped, not reviewed" straight off the verdict, with no separate
// flag needed.
func Bootstrap(ctx context.Context, root string) (Entry, bool, error) {
	existing, found, err := Read(root)
	if err != nil {
		return Entry{}, false, err
	}
	if found {
		return existing, false, nil
	}

	out, err := (wsgit.ExecRunner{}).RunGit(ctx, root, "rev-parse", "HEAD")
	if err != nil {
		return Entry{}, false, fmt.Errorf("resolve HEAD for review ledger bootstrap: %w", err)
	}
	head := strings.TrimSpace(string(out))

	entry := Entry{Base: head, Head: head, Verdict: VerdictBootstrap}
	if err := Append(root, entry); err != nil {
		return Entry{}, false, err
	}
	return entry, true, nil
}
