package mcp

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// git.merge's source-file size warning knobs: built-in keys outside any
// manifest namespace, validated like the worktree nudge knobs.
var (
	gitMergeSizeSoftLinesKey = declaredConfigKey{
		Key:          wsconfig.ItemGitMergeSizeSoftLines,
		Type:         declaredTypeInteger,
		Minimum:      int64Ptr(0),
		Description:  "Soft line limit for git.merge's advisory size warning: a merged impl/* or goal/* branch that pushes a source file from at most this many lines to more than this many gets a warning in the merge result. 0 disables the soft rule.",
		DefaultScope: wsconfig.ScopeProject,
		Default:      "1000",
		DefaultValue: int64(1000),
	}
	gitMergeSizeHardLinesKey = declaredConfigKey{
		Key:          wsconfig.ItemGitMergeSizeHardLines,
		Type:         declaredTypeInteger,
		Minimum:      int64Ptr(0),
		Description:  "Hard line limit for git.merge's advisory size warning: a merged impl/* or goal/* branch that edited a source file which has at least this many lines after the merge gets a warning in the merge result. 0 disables the hard rule.",
		DefaultScope: wsconfig.ScopeProject,
		Default:      "2000",
		DefaultValue: int64(2000),
	}
	gitMergeSizeExtensionsKey = declaredConfigKey{
		Key:          wsconfig.ItemGitMergeSizeExtensions,
		Type:         declaredTypeString,
		Description:  "Space-separated file extensions, without dots, that git.merge's advisory size warning checks; an empty value disables the check.",
		DefaultScope: wsconfig.ScopeProject,
		Default:      defaultMergeSizeExtensions,
		DefaultValue: defaultMergeSizeExtensions,
	}
)

const defaultMergeSizeExtensions = "go js mjs cjs ts tsx jsx py rs java kt c cc cpp h hpp cs swift rb php css scss html vue svelte sh"

// gitMergeSizeKeys lists the size knobs for every built-in registration site.
var gitMergeSizeKeys = []declaredConfigKey{gitMergeSizeSoftLinesKey, gitMergeSizeHardLinesKey, gitMergeSizeExtensionsKey}

// mergeSizeGuard is the resolved size-warning configuration. The zero value
// disables every rule.
type mergeSizeGuard struct {
	Soft, Hard int64
	Extensions map[string]struct{}
}

func newMergeSizeGuard(soft, hard int64, extensions string) mergeSizeGuard {
	g := mergeSizeGuard{Soft: soft, Hard: hard, Extensions: map[string]struct{}{}}
	for _, ext := range strings.Fields(extensions) {
		if ext = strings.ToLower(strings.TrimLeft(ext, ".")); ext != "" {
			g.Extensions[ext] = struct{}{}
		}
	}
	return g
}

func defaultMergeSizeGuard() mergeSizeGuard {
	return newMergeSizeGuard(gitMergeSizeSoftLinesKey.DefaultValue.(int64), gitMergeSizeHardLinesKey.DefaultValue.(int64), defaultMergeSizeExtensions)
}

// resolveMergeSizeGuard reads the size knobs through the layered config; a
// value that does not parse (hand-edited config) falls back to its default.
func resolveMergeSizeGuard(resolver *wsconfig.Resolver, sessionKey string) mergeSizeGuard {
	readInt := func(k declaredConfigKey) int64 {
		def := k.DefaultValue.(int64)
		rv, err := resolver.Get(sessionKey, k.Key)
		if err != nil {
			return def
		}
		n, err := strconv.ParseInt(strings.TrimSpace(rv.Value), 10, 64)
		if err != nil || n < *k.Minimum {
			return def
		}
		return n
	}
	extensions := defaultMergeSizeExtensions
	if rv, err := resolver.Get(sessionKey, gitMergeSizeExtensionsKey.Key); err == nil {
		extensions = rv.Value
	}
	return newMergeSizeGuard(readInt(gitMergeSizeSoftLinesKey), readInt(gitMergeSizeHardLinesKey), extensions)
}

func (g mergeSizeGuard) covers(file string) bool {
	ext := strings.TrimPrefix(path.Ext(file), ".")
	if ext == "" {
		return false
	}
	_, ok := g.Extensions[strings.ToLower(ext)]
	return ok
}

// mergeSizeGrepChunk bounds the paths passed to one git invocation so a large
// merge stays under the platform's argument-length limit.
const mergeSizeGrepChunk = 400

// mergeSizeWarnings computes the advisory size diagnostics for a landed merge
// commit whose second parent is sourceOID and whose pre-merge target was
// targetOID. Files are those the source edited since the merge base. It never
// fails the merge: a Git failure returns one advisory diagnostic saying the
// check could not run, and a file whose counts cannot be read is skipped.
func mergeSizeWarnings(ctx context.Context, root string, runner wsgit.Runner, guard mergeSizeGuard, targetOID, sourceOID, mergeCommit string) []implMergeDiagnostic {
	if (guard.Soft <= 0 && guard.Hard <= 0) || len(guard.Extensions) == 0 {
		return nil
	}
	run := func(dir string, args ...string) (string, error) {
		out, err := runner.RunGit(ctx, dir, args...)
		return string(out), err
	}
	failed := func(step string, err error) []implMergeDiagnostic {
		return []implMergeDiagnostic{{
			Code:           "size_check_failed",
			Classification: "advisory",
			Reason:         fmt.Sprintf("Merge landed, but the source-file size check could not run (%s): %v", step, err),
			Resolution:     "No action is needed to complete this merge; size warnings for it are unavailable.",
		}}
	}
	top, err := run(root, "rev-parse", "--show-toplevel")
	if err != nil {
		return failed("rev-parse --show-toplevel", err)
	}
	top = strings.TrimSpace(top)
	base, err := run(top, "merge-base", targetOID, sourceOID)
	if err != nil {
		return failed("merge-base", err)
	}
	base = strings.TrimSpace(base)
	diff, err := run(top, "-c", "core.quotePath=false", "diff", "--name-status", "-z", "--find-renames", "--diff-filter=AMR", base, sourceOID)
	if err != nil {
		return failed("diff", err)
	}
	// newPath -> path at the merge base ("" when the branch added the file).
	edited := map[string]string{}
	var order []string
	fields := strings.Split(diff, "\x00")
	for i := 0; i < len(fields); {
		status := fields[i]
		// The runner merges stderr into the output, so a stray Git warning
		// can sit between records; skip any field that is not a status.
		if !validNameStatus(status) {
			i++
			continue
		}
		if status[0] == 'R' {
			if i+2 >= len(fields) {
				break
			}
			oldPath, newPath := fields[i+1], fields[i+2]
			i += 3
			if guard.covers(newPath) {
				edited[newPath] = oldPath
				order = append(order, newPath)
			}
			continue
		}
		if i+1 >= len(fields) {
			break
		}
		file := fields[i+1]
		i += 2
		if !guard.covers(file) {
			continue
		}
		if status[0] == 'A' {
			edited[file] = ""
		} else {
			edited[file] = file
		}
		order = append(order, file)
	}
	if len(order) == 0 {
		return nil
	}

	merged, err := mergeSizeLineCounts(run, top, mergeCommit, order)
	if err != nil {
		return failed("grep merged tree", err)
	}
	var basePaths []string
	for _, file := range order {
		if old := edited[file]; old != "" {
			basePaths = append(basePaths, old)
		}
	}
	baseCounts, err := mergeSizeLineCounts(run, top, base, basePaths)
	if err != nil {
		return failed("grep merge base", err)
	}
	attrs, err := mergeSizeAttributes(run, top, mergeCommit, order)
	if err != nil {
		return failed("check-attr", err)
	}

	var out []implMergeDiagnostic
	for _, file := range order {
		lines, ok := merged[file]
		if !ok {
			// Absent from the merged tree, binary, or empty: nothing to warn.
			continue
		}
		var before int64
		if old := edited[file]; old != "" {
			before = baseCounts[old]
		}
		a := attrs[file]
		if isSetAttr(a["linguist-generated"]) || isSetAttr(a["linguist-vendored"]) {
			continue
		}
		switch guardValue := a["ws-size-guard"]; guardValue {
		case "unset":
			continue
		case "", "unspecified", "set":
			if guard.Hard > 0 && lines >= guard.Hard {
				out = append(out, mergeSizeDiagnostic("size_hard_limit", file, lines,
					fmt.Sprintf("Edited %s at %d lines (>= %d); splitting it is recommended.", file, lines, guard.Hard)))
			} else if guard.Soft > 0 && before <= guard.Soft && lines > guard.Soft {
				out = append(out, mergeSizeDiagnostic("size_soft_limit", file, lines,
					fmt.Sprintf("This change pushed %s past %d lines (now %d); splitting it is recommended.", file, guard.Soft, lines)))
			}
		default:
			limit, perr := strconv.ParseInt(strings.TrimSpace(guardValue), 10, 64)
			if perr != nil || limit < 0 {
				// A malformed per-path limit cannot be honoured; do not guess.
				continue
			}
			if before <= limit && lines > limit {
				out = append(out, mergeSizeDiagnostic("size_soft_limit", file, lines,
					fmt.Sprintf("This change pushed %s past %d lines, its ws-size-guard limit (now %d); splitting it is recommended.", file, limit, lines)))
			}
		}
	}
	return out
}

func validNameStatus(status string) bool {
	if status != "A" && status != "M" && !strings.HasPrefix(status, "R") {
		return false
	}
	for _, r := range status[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isSetAttr(value string) bool {
	return value == "set" || value == "true"
}

func mergeSizeDiagnostic(code, file string, lines int64, reason string) implMergeDiagnostic {
	pattern := gitattributesPattern(file)
	return implMergeDiagnostic{
		Code:           code,
		Classification: "advisory",
		Reason:         reason,
		Resolution: fmt.Sprintf("Do not split this file on your own. Ask the user whether refactoring this project is permitted and whether to do it now. "+
			"If the user declines, record that in a normal commit by adding `%s ws-size-guard=%d` to .gitattributes (warn again only past that size), or `%s -ws-size-guard` to stop warning for this path.",
			pattern, mergeSizeMargin(lines), pattern),
	}
}

// mergeSizeMargin is the suggested per-path limit for a declined split: the
// current size plus 20%, rounded up to a multiple of 100.
func mergeSizeMargin(lines int64) int64 {
	limit := lines + (lines+4)/5
	return (limit + 99) / 100 * 100
}

// gitattributesPattern renders a repository-relative path as a .gitattributes
// pattern matching exactly that path: anchored with a leading slash (a
// slash-less pattern would match at any depth), glob metacharacters escaped,
// and C-style quoted when it holds whitespace, a quote, or a control byte.
func gitattributesPattern(file string) string {
	var b strings.Builder
	b.WriteByte('/')
	for _, r := range file {
		switch r {
		case '*', '?', '[', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	pattern := b.String()
	if !strings.ContainsAny(pattern, " \t\"") && !strings.ContainsFunc(pattern, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return pattern
	}
	var q strings.Builder
	q.WriteByte('"')
	for _, r := range pattern {
		switch {
		case r == '"' || r == '\\':
			q.WriteByte('\\')
			q.WriteRune(r)
		case r == '\t':
			q.WriteString(`\t`)
		case r == '\n':
			q.WriteString(`\n`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&q, `\%03o`, r)
		default:
			q.WriteRune(r)
		}
	}
	q.WriteByte('"')
	return q.String()
}

// mergeSizeLineCounts counts the lines of each path in rev with one git grep
// per chunk. Binary and empty files are absent from the result.
func mergeSizeLineCounts(run func(string, ...string) (string, error), top, rev string, paths []string) (map[string]int64, error) {
	counts := map[string]int64{}
	prefix := rev + ":"
	for start := 0; start < len(paths); start += mergeSizeGrepChunk {
		end := min(start+mergeSizeGrepChunk, len(paths))
		args := append([]string{"--literal-pathspecs", "grep", "--no-color", "-c", "-I", "-z", "-e", "", rev, "--"}, paths[start:end]...)
		out, err := run(top, args...)
		if err != nil {
			// git grep exits 1 when nothing matched (every path binary or empty).
			var exitErr *exec.ExitError
			if out == "" && errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
				continue
			}
			return nil, err
		}
		for _, line := range strings.Split(out, "\n") {
			name, count, ok := strings.Cut(line, "\x00")
			if !ok || !strings.HasPrefix(name, prefix) {
				continue
			}
			n, perr := strconv.ParseInt(strings.TrimSpace(count), 10, 64)
			if perr != nil {
				continue
			}
			counts[strings.TrimPrefix(name, prefix)] = n
		}
	}
	return counts, nil
}

// mergeSizeAttributes reads the exclusion attributes of each path from the
// merged tree. A git older than 2.40 refuses --source; the warning is
// advisory, so it then falls back to the working tree, which the merge just
// checked out.
func mergeSizeAttributes(run func(string, ...string) (string, error), top, rev string, paths []string) (map[string]map[string]string, error) {
	attrs := map[string]map[string]string{}
	names := []string{"linguist-generated", "linguist-vendored", "ws-size-guard"}
	for start := 0; start < len(paths); start += mergeSizeGrepChunk {
		end := min(start+mergeSizeGrepChunk, len(paths))
		tail := append(append(append([]string{}, names...), "--"), paths[start:end]...)
		out, err := run(top, append([]string{"check-attr", "-z", "--source", rev}, tail...)...)
		if err != nil {
			out, err = run(top, append([]string{"check-attr", "-z"}, tail...)...)
			if err != nil {
				return nil, err
			}
		}
		fields := strings.Split(out, "\x00")
		for i := 0; i+2 < len(fields); i += 3 {
			file, name, value := fields[i], fields[i+1], fields[i+2]
			if attrs[file] == nil {
				attrs[file] = map[string]string{}
			}
			attrs[file][name] = value
		}
	}
	return attrs, nil
}
