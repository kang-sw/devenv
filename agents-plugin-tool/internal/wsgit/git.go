package wsgit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	DiffModeFull     = "full"
	DiffModeStat     = "stat"
	DiffModeNameOnly = "name_only"
)

type Runner interface {
	RunGit(ctx context.Context, root string, args ...string) ([]byte, error)
}

// ExecRunner runs the user's git. Environ, when set, is the base
// environment of every run (the caller process's, for a server the
// dashboard daemon hosts for another process); nil means this process's.
// Env, when set, is appended to it (a caller that must pin a GIT_* variable,
// such as a ledger that forbids lazy fetches, sets it here).
type ExecRunner struct {
	Environ func() []string
	Env     []string
}

func (r ExecRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	argv := append([]string{"-C", root}, args...)
	var cmd *exec.Cmd
	if r.Environ != nil {
		// The git binary comes from the caller's PATH too (see LookPathEnv);
		// a PATH without git fails the run rather than running this
		// process's git.
		base := r.Environ()
		bin, lookErr := LookPathEnv("git", base)
		cmd = exec.CommandContext(ctx, bin, argv...)
		if lookErr != nil {
			cmd.Err = lookErr
		}
		cmd.Env = append(base, r.Env...)
	} else {
		cmd = exec.CommandContext(ctx, Executable(), argv...)
		if len(r.Env) > 0 {
			cmd.Env = append(os.Environ(), r.Env...)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

// Verifier is an optional structural hook Client.Commit invokes after staging
// and status validation, before the commit lands, so a caller can veto a
// commit whose staged paths fail a domain-specific check (e.g. ticket-file
// well-formedness). wsgit intentionally stays free of an internal/wsdoc
// import here — the dependency runs wsdoc -> wsgit and must not cycle back;
// a caller that needs ticket
// verification wires wsdoc.TicketVerify in through an adapter that produces
// this two-return-value shape, mirroring how Runner keeps wsgit free of an
// os/exec-specific dependency at the type level. The first return value
// carries non-blocking advisory text lines (pre-formatted, text-mode,
// order-preserving, no path/guardrail attribution baked into the type) that
// Client.Commit threads into CommitResult.Advisories without affecting the
// commit outcome. The second return value is the veto: a non-nil error stops
// Client.Commit before the commit lands, and in that case the advisories
// returned alongside it are discarded — a veto has nothing to advise on.
type Verifier func(root string, paths []string) ([]string, error)

type Client struct {
	Runner   Runner
	Verifier Verifier
	// MintID mints a follow-up id; nil uses wskey.Generate. Tests inject a
	// deterministic generator.
	MintID func() (string, error)
}

func NewClient() Client { return Client{Runner: ExecRunner{}} }

// CurrentUserEmail returns the repository's configured git user.email with
// surrounding whitespace trimmed (original case preserved for display). It
// returns "" when git reports no identity — user.email unset, or a CI/bot with
// none — because any error is folded into "": an ownership gate that cannot
// read an identity must degrade to assign-any, never surface a git error to a
// caller that only asked to list tickets. A nil runner falls back to
// ExecRunner, matching Client.runner.
func CurrentUserEmail(ctx context.Context, runner Runner, root string) string {
	if runner == nil {
		runner = ExecRunner{}
	}
	out, err := runner.RunGit(ctx, root, "config", "user.email")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (c Client) runner() Runner {
	if c.Runner == nil {
		return ExecRunner{}
	}
	return c.Runner
}

func (c Client) verifier() Verifier {
	if c.Verifier == nil {
		return func(string, []string) ([]string, error) { return nil, nil }
	}
	return c.Verifier
}
