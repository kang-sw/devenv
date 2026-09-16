package wsrationale

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

// siteChainCap bounds the `git log -L` chain length.
const siteChainCap = 500

// parsedSite is a resolved site address: a file path and a 1-based inclusive
// line range.
type parsedSite struct {
	path       string
	start      int
	end        int
	matchCount int  // regex form only; how many locations the regex matched
	regexForm  bool // whether the address used the regex form
}

// parseSite resolves a `site` argument into a concrete line range, reading the
// file at HEAD when the address is the regex form. occurrence selects which
// regex match to use (1-based); default 1.
func parseSite(ctx context.Context, runner wsgit.Runner, root, site string, occurrence int) (parsedSite, error) {
	site = strings.TrimSpace(site)
	if site == "" {
		return parsedSite{}, fmt.Errorf("site may not be empty")
	}
	if idx := strings.Index(site, "#L"); idx >= 0 {
		return parseSiteLineRef(site, idx)
	}
	return parseSiteRegex(ctx, runner, root, site, occurrence)
}

// parseSiteLineRef parses `<path>#L<start>-L<end>` or `<path>#L<start>`.
func parseSiteLineRef(site string, idx int) (parsedSite, error) {
	path := site[:idx]
	spec := site[idx+2:] // after "#L"
	if path == "" {
		return parsedSite{}, fmt.Errorf("malformed site: missing path in %q", site)
	}
	startStr, endStr, hasEnd := strings.Cut(spec, "-L")
	start, err := strconv.Atoi(strings.TrimSpace(startStr))
	if err != nil || start < 1 {
		return parsedSite{}, fmt.Errorf("malformed site: bad start line in %q", site)
	}
	end := start
	if hasEnd {
		end, err = strconv.Atoi(strings.TrimSpace(endStr))
		if err != nil || end < start {
			return parsedSite{}, fmt.Errorf("malformed site: bad end line in %q", site)
		}
	}
	return parsedSite{path: path, start: start, end: end}, nil
}

// spanRE extracts the `+N` span from the tail of a regex-form site address.
var spanRE = regexp.MustCompile(`\+(\d+)`)

// parseSiteRegex parses `<path>:/<regex>/,+N`, reads the file at HEAD, counts
// matches, and resolves the occurrence-th to a line range.
func parseSiteRegex(ctx context.Context, runner wsgit.Runner, root, site string, occurrence int) (parsedSite, error) {
	path, rest, ok := strings.Cut(site, ":")
	if !ok || path == "" {
		return parsedSite{}, fmt.Errorf("malformed site: expected <path>:/<regex>/,+N or <path>#L<start>-L<end>, got %q", site)
	}
	if !strings.HasPrefix(rest, "/") {
		return parsedSite{}, fmt.Errorf("malformed site: regex must be delimited by / in %q", site)
	}
	lastSlash := strings.LastIndex(rest, "/")
	if lastSlash == 0 {
		return parsedSite{}, fmt.Errorf("malformed site: unterminated regex in %q", site)
	}
	pattern := rest[1:lastSlash]
	tail := rest[lastSlash+1:]
	span := 10
	if m := spanRE.FindStringSubmatch(tail); m != nil {
		span, _ = strconv.Atoi(m[1])
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return parsedSite{}, fmt.Errorf("malformed site: bad regex %q: %w", pattern, err)
	}

	content, err := runner.RunGit(ctx, root, "show", "HEAD:"+path)
	if err != nil {
		return parsedSite{}, fmt.Errorf("site path %q not found at HEAD", path)
	}
	lines := strings.Split(string(content), "\n")
	var matchLines []int
	for i, line := range lines {
		if re.MatchString(line) {
			matchLines = append(matchLines, i+1)
		}
	}
	if len(matchLines) == 0 {
		return parsedSite{}, fmt.Errorf("site regex %q matched no lines in %s", pattern, path)
	}
	if occurrence < 1 {
		occurrence = 1
	}
	if occurrence > len(matchLines) {
		return parsedSite{}, fmt.Errorf("site regex %q matched %d locations; occurrence %d out of range", pattern, len(matchLines), occurrence)
	}
	start := matchLines[occurrence-1]
	end := start + span
	if end > len(lines) {
		end = len(lines)
	}
	return parsedSite{path: path, start: start, end: end, matchCount: len(matchLines), regexForm: true}, nil
}

// scanSiteChain runs `git log -L <start>,<end>:<path> -w --no-patch` bounded by
// the chain cap and parses the sentinel stream. The returned commits are the
// whole chain, newest first.
func scanSiteChain(ctx context.Context, runner wsgit.Runner, root string, site parsedSite) (commits []rawCommit, truncated bool, err error) {
	lval := fmt.Sprintf("-L%d,%d:%s", site.start, site.end, site.path)
	args := []string{
		"log",
		"--date=short",
		"-w",
		"--no-patch",
		logFormat(),
		"-n", strconv.Itoa(siteChainCap + 1),
		lval,
	}
	out, err := runner.RunGit(ctx, root, args...)
	if err != nil {
		return nil, false, err
	}
	commits = parseCommitStream(out)
	if len(commits) > siteChainCap {
		commits = commits[:siteChainCap]
		truncated = true
	}
	// -L does not carry --name-only; the tracked file is the one touched path
	// every chain commit shares.
	for i := range commits {
		commits[i].paths = []string{site.path}
	}
	return commits, truncated, nil
}
