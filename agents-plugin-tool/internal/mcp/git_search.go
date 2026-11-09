package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// git.search is a hosted agent's read-only window into the user's local
// repositories. It is an allowlist, not a denylist: each subcommand accepts
// only the flags listed below, and every other argument shape is refused,
// because git has too many ways to write a file, run a program, or leave the
// repository (--output, external diff and textconv drivers, pagers,
// -c/-C/--git-dir) for a denylist to stay complete.

const (
	gitSearchTimeout   = 20 * time.Second
	gitSearchOutputCap = 64 << 10
	gitSearchStderrCap = 8 << 10
)

var gitSearchCommands = []string{"log", "show", "grep", "blame", "ls-files"}

// gitSearchForcedFlags follow the subcommand on every call, before the
// caller's options. Each was checked against git 2.50: grep refuses
// --no-ext-diff and --no-show-signature, blame finds --no-color ambiguous
// (--no-color-lines, --no-color-by-age), and ls-files takes none of them.
var gitSearchForcedFlags = map[string][]string{
	"log":      {"--no-ext-diff", "--no-textconv", "--no-color", "--no-show-signature"},
	"show":     {"--no-ext-diff", "--no-textconv", "--no-color", "--no-show-signature"},
	"grep":     {"--no-textconv", "--no-color"},
	"blame":    {"--no-ext-diff", "--no-textconv"},
	"ls-files": nil,
}

// gitSearchValueCheck validates the value of a --name=value or attached
// short flag, returning what is wrong with it, or "".
type gitSearchValueCheck func(value string) string

var (
	gitSearchDigitsPattern = regexp.MustCompile(`^[0-9]+$`)
	gitSearchScorePattern  = regexp.MustCompile(`^[0-9]+%?$`)
)

func gitSearchAnyValue(value string) string {
	if value == "" {
		return "needs a value"
	}
	return ""
}

func gitSearchDigits(value string) string {
	if !gitSearchDigitsPattern.MatchString(value) {
		return "takes a whole number"
	}
	return ""
}

func gitSearchScore(value string) string {
	if !gitSearchScorePattern.MatchString(value) {
		return "takes a similarity score such as 50 or 50%"
	}
	return ""
}

// gitSearchPrettyNames are the built-in --pretty/--format names. Any other
// bare name is a pretty.<name> alias from the repository's config, whose
// expansion this tool cannot see, so it is refused.
var gitSearchPrettyNames = map[string]bool{
	"oneline": true, "short": true, "medium": true, "full": true, "fuller": true,
	"reference": true, "email": true, "mboxrd": true, "raw": true,
}

// gitSearchPrettyFormat checks a log or show --pretty/--format value. %G
// placeholders verify signatures, which runs the configured gpg program.
func gitSearchPrettyFormat(value string) string {
	if value == "" {
		return "needs a value"
	}
	if strings.Contains(value, "%G") {
		return "uses a %G signature placeholder, which runs the signature-verification program"
	}
	if strings.HasPrefix(value, "format:") || strings.HasPrefix(value, "tformat:") || strings.Contains(value, "%") {
		return ""
	}
	if gitSearchPrettyNames[value] {
		return ""
	}
	return "names neither a built-in format (oneline, short, medium, full, fuller, reference, email, raw) nor a format: string"
}

// gitSearchFlagSet is one subcommand's flag allowlist.
type gitSearchFlagSet struct {
	// exact are flags accepted only as written, with no value.
	exact []string
	// valued are long flags accepted as --name=value.
	valued map[string]gitSearchValueCheck
	// short are short flags accepted with an attached value, -X<value>.
	short map[string]gitSearchValueCheck
	// numeric accepts -<N> (log: a commit count; grep: context lines).
	numeric bool
	// refused names flags this subcommand refuses for a stated reason; a
	// single-letter short flag refuses every attached form too.
	refused map[string]string
}

func gitSearchMerge(maps ...map[string]gitSearchValueCheck) map[string]gitSearchValueCheck {
	out := map[string]gitSearchValueCheck{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// gitSearchDiffExact and gitSearchDiffValued are the output and diff flags
// log and show share.
var gitSearchDiffExact = []string{
	"-p", "-u", "--patch", "-s", "--no-patch", "--raw", "--stat", "--shortstat", "--numstat",
	"--compact-summary", "--summary", "--name-only", "--name-status", "--patch-with-stat",
	"--patch-with-raw", "--full-index", "--binary", "--no-prefix", "--abbrev", "--abbrev-commit",
	"--no-abbrev-commit", "--oneline", "--minimal", "--patience", "--histogram", "-w",
	"--ignore-all-space", "-b", "--ignore-space-change", "--ignore-space-at-eol",
	"--ignore-blank-lines", "-W", "--function-context", "-M", "--find-renames", "-C",
	"--find-copies", "--find-copies-harder", "--no-renames", "--text", "-a", "--relative",
	"--no-relative", "--notes", "--no-notes", "--decorate", "--no-decorate", "-m", "--cc",
	"--first-parent", "--pickaxe-all", "--pickaxe-regex", "--word-diff", "--irreversible-delete",
	"-D", "-R", "--dirstat", "--cumulative", "--parents", "--show-notes",
}

var gitSearchDiffValued = map[string]gitSearchValueCheck{
	"--format": gitSearchPrettyFormat, "--pretty": gitSearchPrettyFormat,
	"--date": gitSearchAnyValue, "--abbrev": gitSearchDigits, "--stat": gitSearchAnyValue,
	"--dirstat": gitSearchAnyValue, "--diff-filter": gitSearchAnyValue,
	"--find-renames": gitSearchScore, "--find-copies": gitSearchScore,
	"--word-diff": gitSearchAnyValue, "--word-diff-regex": gitSearchAnyValue,
	"--relative": gitSearchAnyValue, "--decorate": gitSearchAnyValue,
	"--diff-merges": gitSearchAnyValue, "--unified": gitSearchDigits,
	"--inter-hunk-context": gitSearchDigits, "--diff-algorithm": gitSearchAnyValue,
	"--notes": gitSearchAnyValue, "--show-notes": gitSearchAnyValue,
	"--encoding": gitSearchAnyValue, "--find-object": gitSearchAnyValue,
	"--anchored": gitSearchAnyValue, "--ignore-matching-lines": gitSearchAnyValue,
	"--stat-width": gitSearchDigits, "--stat-name-width": gitSearchDigits,
	"--stat-graph-width": gitSearchDigits, "--stat-count": gitSearchDigits,
	"--src-prefix": gitSearchAnyValue, "--dst-prefix": gitSearchAnyValue,
	"--line-prefix": gitSearchAnyValue,
}

var gitSearchDiffShort = map[string]gitSearchValueCheck{
	"-U": gitSearchDigits, "-M": gitSearchScore, "-C": gitSearchScore, "-l": gitSearchDigits,
	"-S": gitSearchAnyValue, "-G": gitSearchAnyValue, "-I": gitSearchAnyValue,
}

// gitSearchCommonRefused applies to every subcommand: each would write a
// file, run a program, or show signatures (which runs gpg).
var gitSearchCommonRefused = map[string]string{
	"--output":         "writes its output to a file",
	"--ext-diff":       "runs an external diff program",
	"--textconv":       "runs textconv filter programs",
	"--color":          "colours the output for a terminal",
	"--color-words":    "colours the output for a terminal",
	"--color-moved":    "colours the output for a terminal",
	"--show-signature": "runs the signature-verification program",
	"--remerge-diff":   "writes temporary objects into the repository",
	"--stdin":          "reads standard input, which this tool does not give",
	"--no-index":       "searches outside the repository",
	"-O":               "runs a program or reads a file outside the repository",
	"--orderfile":      "reads a file outside the repository",
}

var gitSearchFlagSets = map[string]gitSearchFlagSet{
	"log": {
		exact: append(append([]string{}, gitSearchDiffExact...),
			"--all", "--branches", "--tags", "--remotes", "--graph", "--reverse", "--no-merges",
			"--merges", "--follow", "--full-history", "--simplify-merges", "--simplify-by-decoration",
			"--ancestry-path", "--topo-order", "--date-order", "--author-date-order", "--left-right",
			"--cherry-pick", "--cherry-mark", "--boundary", "--source", "--root", "--all-match",
			"--invert-grep", "-i", "--regexp-ignore-case", "-E", "--extended-regexp", "-F",
			"--fixed-strings", "-P", "--perl-regexp", "--basic-regexp", "--full-diff", "--no-walk",
			"--do-walk", "--children", "--use-mailmap", "--mailmap", "--log-size"),
		valued: gitSearchMerge(gitSearchDiffValued, map[string]gitSearchValueCheck{
			"--max-count": gitSearchDigits, "--skip": gitSearchDigits, "--since": gitSearchAnyValue,
			"--after": gitSearchAnyValue, "--until": gitSearchAnyValue, "--before": gitSearchAnyValue,
			"--since-as-filter": gitSearchAnyValue, "--author": gitSearchAnyValue,
			"--committer": gitSearchAnyValue, "--grep": gitSearchAnyValue,
			"--branches": gitSearchAnyValue, "--tags": gitSearchAnyValue, "--remotes": gitSearchAnyValue,
			"--exclude": gitSearchAnyValue, "--min-parents": gitSearchDigits,
			"--max-parents": gitSearchDigits, "--ancestry-path": gitSearchAnyValue,
			"--no-walk": gitSearchAnyValue,
		}),
		short: gitSearchMerge(gitSearchDiffShort, map[string]gitSearchValueCheck{
			"-n": gitSearchDigits, "-L": gitSearchAnyValue,
		}),
		numeric: true,
	},
	"show": {
		exact:  gitSearchDiffExact,
		valued: gitSearchDiffValued,
		short:  gitSearchDiffShort,
	},
	"grep": {
		exact: []string{
			"-n", "--line-number", "-i", "--ignore-case", "-w", "--word-regexp", "-v",
			"--invert-match", "-E", "--extended-regexp", "-F", "--fixed-strings", "-G",
			"--basic-regexp", "-P", "--perl-regexp", "-l", "--files-with-matches", "--name-only",
			"-L", "--files-without-match", "-c", "--count", "--cached", "--untracked",
			"--exclude-standard", "--no-exclude-standard", "-h", "-H", "--full-name", "-o",
			"--only-matching", "--column", "-I", "-a", "--text", "-z", "--null", "--break",
			"--heading", "-p", "--show-function", "-W", "--function-context", "-r", "--recursive",
			"--no-recursive", "--all-match",
		},
		valued: map[string]gitSearchValueCheck{
			"--max-depth": gitSearchDigits, "--max-count": gitSearchDigits,
			"--context": gitSearchDigits, "--before-context": gitSearchDigits,
			"--after-context": gitSearchDigits,
		},
		short: map[string]gitSearchValueCheck{
			"-A": gitSearchDigits, "-B": gitSearchDigits, "-C": gitSearchDigits, "-m": gitSearchDigits,
		},
		numeric: true,
		refused: map[string]string{
			"--open-files-in-pager": "runs a pager program",
			"-f":                    "reads patterns from a file; pass the pattern argument instead",
			"-e":                    "is set from the pattern argument",
		},
	},
	"blame": {
		exact: []string{
			"-w", "-M", "-C", "-p", "--porcelain", "--line-porcelain", "-l", "-t", "-s", "-e",
			"--show-email", "-n", "--show-number", "-f", "--show-name", "-b", "--root",
			"--minimal", "--incremental", "--abbrev", "--reverse", "--first-parent", "--show-stats",
		},
		valued: map[string]gitSearchValueCheck{
			"--abbrev": gitSearchDigits, "--ignore-rev": gitSearchAnyValue,
			"--date": gitSearchAnyValue,
		},
		short: map[string]gitSearchValueCheck{
			"-L": gitSearchAnyValue, "-M": gitSearchScore, "-C": gitSearchScore,
		},
		refused: map[string]string{
			"--contents":         "reads a file outside the repository",
			"-S":                 "reads revisions from a file",
			"--ignore-revs-file": "reads a file outside the repository",
		},
	},
	"ls-files": {
		exact: []string{
			"-c", "--cached", "-d", "--deleted", "-m", "--modified", "-o", "--others", "-i",
			"--ignored", "-s", "--stage", "-u", "--unmerged", "-k", "--killed", "-t", "-v", "-f",
			"-z", "--directory", "--no-empty-directory", "--empty-directory", "--eol",
			"--full-name", "--exclude-standard", "--deduplicate", "--abbrev", "--sparse",
			"--error-unmatch",
		},
		valued: map[string]gitSearchValueCheck{
			"--format": gitSearchAnyValue, "--exclude": gitSearchAnyValue, "--abbrev": gitSearchDigits,
		},
		short: map[string]gitSearchValueCheck{"-x": gitSearchAnyValue},
		refused: map[string]string{
			"-X":                      "reads exclude patterns from a file outside the repository",
			"--exclude-from":          "reads exclude patterns from a file outside the repository",
			"--exclude-per-directory": "reads exclude patterns from files this tool does not choose",
			"--with-tree":             "takes a revision, and ls-files takes none here",
			"--recurse-submodules":    "reads other repositories",
		},
	},
}

// gitSearchGlobalLong are git's own options (before the subcommand), which
// would point git at another repository, configuration, or program.
var gitSearchGlobalLong = []string{"--git-dir", "--work-tree", "--exec-path", "--namespace", "--config-env", "--super-prefix", "--list-cmds"}

// gitSearchGlobalProblem reports opts[i] when it is a git global option:
// a global long option, -c name=value, or -C <dir>. Grep's -c and -C<n>
// and blame's -C[<score>] stay allowed: as global options both take a
// separate argument or an attached name=value or path.
func gitSearchGlobalProblem(opts []string, i int) bool {
	o := opts[i]
	name, _, _ := strings.Cut(o, "=")
	for _, g := range gitSearchGlobalLong {
		if name == g {
			return true
		}
	}
	if o == "-c" || o == "-C" {
		return i+1 < len(opts) && !strings.HasPrefix(opts[i+1], "-")
	}
	if strings.HasPrefix(o, "-c") && strings.Contains(o, "=") {
		return true
	}
	if strings.HasPrefix(o, "-C") && !strings.HasPrefix(o, "--") && !gitSearchScorePattern.MatchString(o[2:]) {
		return true
	}
	return false
}

// gitSearchFlagProblem reports what is wrong with one option of command, or
// "" when the allowlist accepts it.
func gitSearchFlagProblem(command string, opts []string, i int) string {
	o := opts[i]
	set := gitSearchFlagSets[command]
	if !strings.HasPrefix(o, "-") || o == "-" {
		return fmt.Sprintf("%q is not a flag: options holds flags only, with any value attached (--name=value, -n5); revisions, paths, and the grep pattern have their own arguments", o)
	}
	if o == "--" || o == "--end-of-options" {
		return fmt.Sprintf("%q is placed by the tool itself", o)
	}
	if gitSearchGlobalProblem(opts, i) {
		return fmt.Sprintf("%q is a git global option; this tool runs git in the repo you pass and takes no git configuration, directory, or program options", o)
	}
	long := strings.HasPrefix(o, "--")
	name, value, hasValue := o, "", false
	if long {
		name, value, hasValue = strings.Cut(o, "=")
	}
	for _, reasons := range []map[string]string{set.refused, gitSearchCommonRefused} {
		if long {
			if reason, ok := reasons[name]; ok {
				return fmt.Sprintf("%s %s", name, reason)
			}
			continue
		}
		if reason, ok := reasons[o[:2]]; ok {
			return fmt.Sprintf("%s %s", o[:2], reason)
		}
	}
	if long && (name == "--format" || name == "--pretty") && hasValue && strings.Contains(value, "%G") {
		return fmt.Sprintf("%s=%s uses a %%G signature placeholder, which runs the signature-verification program", name, value)
	}
	for _, flag := range set.exact {
		if o == flag {
			return ""
		}
	}
	if long {
		check, ok := set.valued[name]
		if ok && hasValue {
			if problem := check(value); problem != "" {
				return fmt.Sprintf("%s %s", name, problem)
			}
			return ""
		}
		if ok {
			return fmt.Sprintf("%s takes a value; write it as %s=<value>", name, name)
		}
		return fmt.Sprintf("%s is not accepted by %s", o, command)
	}
	if set.numeric && gitSearchDigitsPattern.MatchString(o[1:]) {
		return ""
	}
	if check, ok := set.short[o[:2]]; ok {
		if len(o) == 2 {
			return fmt.Sprintf("%s takes an attached value; write it as %s<value>", o, o)
		}
		if problem := check(o[2:]); problem != "" {
			return fmt.Sprintf("%s %s", o[:2], problem)
		}
		return ""
	}
	return fmt.Sprintf("%s is not accepted by %s", o, command)
}

// gitSearchAcceptedFlags lists command's allowlist for a refusal.
func gitSearchAcceptedFlags(command string) string {
	set := gitSearchFlagSets[command]
	seen := map[string]bool{}
	var flags []string
	add := func(f string) {
		if !seen[f] {
			seen[f] = true
			flags = append(flags, f)
		}
	}
	for _, f := range set.exact {
		add(f)
	}
	for f := range set.valued {
		add(f + "=<value>")
	}
	for f := range set.short {
		add(f + "<value>")
	}
	sort.Strings(flags)
	if set.numeric {
		flags = append(flags, "-<N>")
	}
	return strings.Join(flags, " ")
}

// gitSearchRoot is one directory git.search may read, canonical.
type gitSearchRoot struct {
	path   string
	source string
}

// gitSearchRoots is every directory git.search may read: the project root
// of each session the dashboard registry saw within its retention window
// (agent-runner conversations excluded, since their root is a store clone),
// then each agent.git_search_roots entry. ignored names the knob entries
// that were dropped, with why.
func (s *Server) gitSearchRoots(sessionKey string) (roots []gitSearchRoot, ignored []string) {
	seen := map[string]bool{}
	add := func(path, source string) {
		if !seen[path] {
			seen[path] = true
			roots = append(roots, gitSearchRoot{path: path, source: source})
		}
	}
	if dir, err := wsdashboard.Dir(""); err == nil {
		regs, _ := wsdashboard.ReadRegistrations(dir)
		now := time.Now()
		var projects []string
		for _, reg := range regs {
			if reg.Agent {
				continue
			}
			// ReadRegistrations returns every entry still on disk; a
			// session last seen past the retention window no longer counts.
			last := reg.LastSeen()
			if last.IsZero() || now.Sub(last) > wsdashboard.RegistryRetention {
				continue
			}
			if path, ok := gitSearchExistingDir(reg.Project); ok {
				projects = append(projects, path)
			}
		}
		sort.Strings(projects)
		for _, path := range projects {
			add(path, "session")
		}
	}
	adapter := sessionConfigAdapter{s: s.sessions}
	resolver := wsconfig.NewResolver(s.sessionConfigOptions(sessionKey), builtinConfigDefaults(), adapter, adapter)
	rv, err := resolver.Get(sessionKey, wsconfig.ItemAgentGitSearchRoots)
	if err != nil {
		ignored = append(ignored, fmt.Sprintf("%s unreadable: %v", wsconfig.ItemAgentGitSearchRoots, err))
		return roots, ignored
	}
	for _, entry := range filepath.SplitList(rv.Value) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !filepath.IsAbs(entry) {
			ignored = append(ignored, entry+": not an absolute path")
			continue
		}
		path, ok := gitSearchExistingDir(entry)
		if !ok {
			ignored = append(ignored, entry+": does not exist or is not a directory")
			continue
		}
		add(path, wsconfig.ItemAgentGitSearchRoots)
	}
	return roots, ignored
}

// gitSearchExistingDir is path made absolute with every symlink resolved,
// when it names an existing directory.
func gitSearchExistingDir(path string) (string, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return filepath.Clean(resolved), true
}

// gitSearchWithin reports whether path equals root or sits inside it; both
// are canonical.
func gitSearchWithin(path, root string) bool {
	if path == root {
		return true
	}
	prefix := root
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
}

func formatGitSearchRoots(roots []gitSearchRoot, ignored []string) string {
	var sb strings.Builder
	if len(roots) == 0 {
		fmt.Fprintf(&sb, "No repository is readable yet. %s reads the project roots of the sessions the dashboard saw on this machine in the last 7 days, and the directories the user lists in the %s config knob.\n", gitSearchTool, wsconfig.ItemAgentGitSearchRoots)
	} else {
		sb.WriteString("Readable roots; pass one as repo, or a directory inside one:\n")
		for _, r := range roots {
			fmt.Fprintf(&sb, "%s (%s)\n", r.path, r.source)
		}
	}
	if len(ignored) > 0 {
		fmt.Fprintf(&sb, "Ignored %s entries:\n", wsconfig.ItemAgentGitSearchRoots)
		for _, line := range ignored {
			sb.WriteString(line + "\n")
		}
	}
	return sb.String()
}

// gitSearchStrings reads an optional string-array argument.
func gitSearchStrings(args map[string]any, name string) ([]string, error) {
	raw, ok := args[name]
	if !ok || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", name)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", name)
		}
		out = append(out, text)
	}
	return out, nil
}

// gitSearchRequest is a validated call.
type gitSearchRequest struct {
	command   string
	options   []string
	pattern   string
	revisions []string
	paths     []string
}

// parseGitSearchRequest validates everything but the repository path.
func parseGitSearchRequest(args map[string]any) (gitSearchRequest, error) {
	var req gitSearchRequest
	req.command, _ = args["command"].(string)
	req.command = strings.TrimSpace(req.command)
	if _, ok := gitSearchFlagSets[req.command]; !ok {
		if req.command == "" {
			return req, fmt.Errorf("command is required with repo: one of %s", strings.Join(gitSearchCommands, ", "))
		}
		return req, fmt.Errorf("command %q is not one of %s", req.command, strings.Join(gitSearchCommands, ", "))
	}
	var err error
	if req.options, err = gitSearchStrings(args, "options"); err != nil {
		return req, err
	}
	if req.revisions, err = gitSearchStrings(args, "revisions"); err != nil {
		return req, err
	}
	if req.paths, err = gitSearchStrings(args, "paths"); err != nil {
		return req, err
	}
	if raw, ok := args["pattern"]; ok && raw != nil {
		p, ok := raw.(string)
		if !ok {
			return req, errors.New("pattern must be a string")
		}
		req.pattern = p
	}
	if req.command == "grep" && req.pattern == "" {
		return req, errors.New("grep needs a pattern")
	}
	if req.command != "grep" && req.pattern != "" {
		return req, fmt.Errorf("pattern is for grep only; %s takes none", req.command)
	}

	var problems []string
	for i := range req.options {
		if problem := gitSearchFlagProblem(req.command, req.options, i); problem != "" {
			problems = append(problems, problem)
		}
	}
	if len(problems) > 0 {
		return req, fmt.Errorf("refused: %s\n%s accepts: %s", strings.Join(problems, "; "), req.command, gitSearchAcceptedFlags(req.command))
	}
	for _, rev := range req.revisions {
		if rev == "" {
			return req, errors.New("a revision is empty")
		}
		if strings.HasPrefix(rev, "-") {
			return req, fmt.Errorf("revision %q starts with -; a flag goes in options", rev)
		}
	}
	for _, path := range req.paths {
		if path == "" {
			return req, errors.New("a path is empty")
		}
	}
	switch req.command {
	case "ls-files":
		if len(req.revisions) > 0 {
			return req, errors.New("ls-files lists the index and takes no revisions; list a revision's files with show --name-only, or log --name-only")
		}
	case "blame":
		if len(req.revisions) > 1 {
			return req, errors.New("blame takes at most one revision")
		}
		if len(req.paths) != 1 {
			return req, errors.New("blame takes exactly one path")
		}
	}
	return req, nil
}

// gitSearchArgs is the git argument list for req. log and show take the
// common shape. After --end-of-options, ls-files reads "--" as a pathspec
// (checked on git 2.50), so its paths go bare (each still a path, even one
// starting with -).
//
// grep and blame get no --end-of-options at all: git 2.43 grep reads it as
// a revision ("unable to resolve revision: --end-of-options"), and 2.43
// blame reads the word after it as its path ("--end-of-options -- f" fails
// with "no such path '--'"), while 2.50 blame instead rejects
// "--end-of-options HEAD -- f". Neither needs it: grep's pattern goes
// through -e, a revision starting with - is refused by the request parser,
// and their paths follow "--", so "[rev] -- path" is safe on both versions.
func gitSearchArgs(req gitSearchRequest) []string {
	args := []string{"--no-pager", "-c", "core.fsmonitor=false", req.command}
	args = append(args, gitSearchForcedFlags[req.command]...)
	args = append(args, req.options...)
	switch req.command {
	case "grep":
		args = append(args, "-e", req.pattern)
	case "blame":
	default:
		args = append(args, "--end-of-options")
	}
	if req.command == "ls-files" {
		return append(args, req.paths...)
	}
	args = append(args, req.revisions...)
	args = append(args, "--")
	return append(args, req.paths...)
}

// gitSearchEnv is the process environment with every GIT_* variable
// dropped, so no inherited GIT_DIR, GIT_CONFIG_*, or GIT_EXTERNAL_DIFF
// steers git, and repository discovery stopped above root. LC_ALL stays
// as inherited: forcing C would break case folding of non-ASCII patterns.
func gitSearchEnv(root string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_") {
			continue
		}
		env = append(env, kv)
	}
	return append(env,
		"GIT_CEILING_DIRECTORIES="+filepath.Dir(root),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
		// A partial clone would fetch a missing object from its remote.
		"GIT_NO_LAZY_FETCH=1",
		"GIT_PAGER=cat",
		"PAGER=cat",
	)
}

// gitSearchBuffer keeps the first limit bytes written and calls full once
// when more arrive, so the caller can stop git instead of draining it.
type gitSearchBuffer struct {
	buf       bytes.Buffer
	limit     int
	truncated bool
	full      func()
}

func (b *gitSearchBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buf.Len(); room < len(p) {
		if room > 0 {
			b.buf.Write(p[:room])
		}
		if !b.truncated {
			b.truncated = true
			if b.full != nil {
				b.full()
			}
		}
		return len(p), nil
	}
	b.buf.Write(p)
	return len(p), nil
}

func (s *Server) handleGitSearch(ctx context.Context, id json.RawMessage, args map[string]any) response {
	const tool = gitSearchTool
	key, err := sessionStateKey(tool, args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	if _, found := s.sessions.lookup(key); !found {
		return toolTextResponse(id, "", mailboxUnknownSessionError(tool))
	}
	roots, ignored := s.gitSearchRoots(key)
	repoArg, _ := args["repo"].(string)
	repoArg = strings.TrimSpace(repoArg)
	if repoArg == "" {
		return toolTextResponse(id, formatGitSearchRoots(roots, ignored), nil)
	}
	if !filepath.IsAbs(repoArg) {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: repo %q is not an absolute path; call %s without repo to list the readable roots", tool, repoArg, tool))
	}
	repo, ok := gitSearchExistingDir(repoArg)
	if !ok {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: repo %s does not exist or is not a directory", tool, repoArg))
	}
	var matched string
	for _, r := range roots {
		// The outermost matching root, so discovery may climb to it.
		if gitSearchWithin(repo, r.path) && (matched == "" || len(r.path) < len(matched)) {
			matched = r.path
		}
	}
	if matched == "" {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: %s is outside every root this tool may read.\n%s", tool, repo, formatGitSearchRoots(roots, ignored)))
	}
	req, err := parseGitSearchRequest(args)
	if err != nil {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: %v", tool, err))
	}
	text, err := runGitSearch(ctx, repo, matched, req)
	if err != nil {
		return toolErrorTextResponse(id, fmt.Sprintf("%s: %v", tool, err))
	}
	return toolTextResponse(id, text, nil)
}

// runGitSearch runs req in repo, discovery bounded by root, and returns its
// output, capped, or git's stderr as the error.
func runGitSearch(ctx context.Context, repo, root string, req gitSearchRequest) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitSearchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", gitSearchArgs(req)...)
	cmd.Dir = repo
	cmd.Env = gitSearchEnv(root)
	stdout := &gitSearchBuffer{limit: gitSearchOutputCap, full: cancel}
	stderr := &gitSearchBuffer{limit: gitSearchStderrCap}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	runErr := cmd.Run()
	out := stdout.buf.String()
	if stdout.truncated {
		if i := strings.LastIndexByte(out, '\n'); i >= 0 {
			out = out[:i+1]
		}
		return out + fmt.Sprintf("[output truncated at %d KiB; narrow it with paths, revisions, or a limit such as -n<N> or --max-count=<N>]\n", gitSearchOutputCap>>10), nil
	}
	if runErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("git %s timed out after %s; narrow it with paths, revisions, or a limit", req.command, gitSearchTimeout)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			if req.command == "grep" && exitErr.ExitCode() == 1 && strings.TrimSpace(out) == "" && strings.TrimSpace(stderr.buf.String()) == "" {
				return "no matches\n", nil
			}
			if msg := strings.TrimSpace(stderr.buf.String()); msg != "" {
				return "", errors.New(msg)
			}
			return "", fmt.Errorf("git %s exited with status %d", req.command, exitErr.ExitCode())
		}
		return "", runErr
	}
	if out == "" {
		return "(no output)\n", nil
	}
	return out, nil
}
