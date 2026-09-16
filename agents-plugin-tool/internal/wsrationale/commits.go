package wsrationale

import (
	"context"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// scanCap bounds a full-history or pickaxe scan. A repository longer than this
// reports truncation rather than paying an unbounded scan cost. v1 has no
// cache; a downstream repository that measures above 2s is the follow-up
// trigger.
const scanCap = 20000

// commitSections are the two commit-body blocks whose bullets and paragraphs
// carry recorded rationale.
var commitSections = []struct {
	heading string
	prefix  string
}{
	{"## AI Context", "## "},
	{"## Ticket Updates", "## "},
}

// sentinel field/record separators, mirroring wsgit's use of control bytes that
// never appear in commit metadata. bodyEnd separates the formatted header+body
// from the --name-only path list that git appends after it.
const (
	recSep   = "\x1e"
	fieldSep = "\x1f"
	bodyEnd  = "\x1d"
)

func logFormat() string {
	return "--pretty=format:" + recSep + "%H" + fieldSep + "%ad" + fieldSep + "%s" + fieldSep + "%b" + bodyEnd
}

type rawCommit struct {
	hash    string
	date    string
	subject string
	body    string
	paths   []string
}

// scanCommits runs a bounded `git log` (optionally a pickaxe `-S` scan) and
// parses the sentinel stream into raw commits, newest first. truncated is true
// when history exceeds the scan cap.
func scanCommits(ctx context.Context, runner wsgit.Runner, root, pickaxe string) (commits []rawCommit, truncated bool, err error) {
	args := []string{
		"log",
		"--date=short",
		"--diff-merges=first-parent",
		"--name-only",
		logFormat(),
		"-n", strconv.Itoa(scanCap + 1),
	}
	if pickaxe != "" {
		args = append(args, "-S"+pickaxe)
	}
	out, err := runner.RunGit(ctx, root, args...)
	if err != nil {
		return nil, false, err
	}
	commits = parseCommitStream(out)
	if len(commits) > scanCap {
		commits = commits[:scanCap]
		truncated = true
	}
	return commits, truncated, nil
}

// parseCommitStream parses a sentinel-delimited `git log --name-only` stream.
func parseCommitStream(out []byte) []rawCommit {
	text := string(out)
	chunks := strings.Split(text, recSep)
	commits := make([]rawCommit, 0, len(chunks))
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			continue
		}
		header, rest, hasBodyEnd := strings.Cut(chunk, bodyEnd)
		fields := strings.SplitN(header, fieldSep, 4)
		if len(fields) < 4 {
			continue
		}
		c := rawCommit{
			hash:    strings.TrimSpace(fields[0]),
			date:    strings.TrimSpace(fields[1]),
			subject: strings.TrimSpace(fields[2]),
			body:    fields[3],
		}
		if hasBodyEnd {
			for _, line := range strings.Split(rest, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					c.paths = append(c.paths, line)
				}
			}
		}
		if c.hash == "" {
			continue
		}
		commits = append(commits, c)
	}
	return commits
}

// commitRecords turns one raw commit into its rationale records. The commit's
// stems (every ticket stem its whole body mentions) attach to each record.
func commitRecords(c rawCommit) []Record {
	stems := extractStems(c.body)
	var records []Record
	for _, sec := range commitSections {
		for _, block := range collectSections(c.body, exactOrPrefixHeading(sec.heading), sec.prefix) {
			for _, text := range splitRecords(block.lines) {
				records = append(records, Record{
					Kind:    "commit",
					Hash:    c.hash,
					Date:    c.date,
					Subject: c.subject,
					Text:    text,
					Paths:   c.paths,
					Stems:   stems,
				})
			}
		}
	}
	return records
}
