package wsagent

import (
	"fmt"
	"slices"
	"strings"

	"github.com/kang-sw/devenv/internal/wsinbox"
	"github.com/kang-sw/devenv/internal/wslive"
)

// PlaybookCommandSource is the Source of a dashboard command the runner
// offers for a lead playbook (Options.PlaybookCommands), not the harness.
const PlaybookCommandSource = "ws"

// NativeSubagentTool is the native subagent tool the profile's
// subagent_removal item names for role, whatever its grade: a tool the
// harness cannot remove is still the one a session must not use. "" when
// the item names none (or p is nil).
func (p *HarnessProfile) NativeSubagentTool(role CatalogRole) string {
	if p == nil {
		return ""
	}
	for _, entry := range p.Entries {
		if entry.Item != ItemSubagentRemoval || (entry.Role != "" && entry.Role != role) {
			continue
		}
		for _, value := range entry.Values {
			if value != "" && value != CatalogAnyValue {
				return value
			}
		}
	}
	return ""
}

// subagentSteer is the line that keeps rec's session off its harness's
// native subagent tool, so it delegates through ws's agent tools alone:
// only under the removal policy (RemovesSubagents, whatever the harness
// can physically remove), and only where the backend's catalog names that
// tool. "" otherwise: a session outside the policy, such as a tracker
// support conversation, gets none, and the CLI tier never reads this text.
func (r *Runner) subagentSteer(rec Record) string {
	if !RemovesSubagents(rec) {
		return ""
	}
	role := RoleWorker
	if rec.Lead {
		role = RoleLead
	}
	tool := r.opts.Backends[rec.Backend].Catalog.NativeSubagentTool(role)
	if tool == "" {
		return ""
	}
	return fmt.Sprintf("Never use the native `%s` tool. Start a subagent only with the ws `agent.spawn` tool, and continue one with the ws `mailbox.send` tool.", tool)
}

// playbookCommands is the lead playbook command list a hosted lead of
// backend offers (Options.PlaybookCommands), nil for a backend without
// BackendRegistration.PlaybookCommands. It is read at the first need and
// kept; a failed read is logged, offers none, and is retried at the next
// need.
func (r *Runner) playbookCommands(backend string) []wslive.SlashCommand {
	if !r.opts.Backends[backend].PlaybookCommands || r.opts.PlaybookCommands == nil {
		return nil
	}
	r.playbookMu.Lock()
	defer r.playbookMu.Unlock()
	if !r.playbookLoaded {
		list, err := r.opts.PlaybookCommands()
		if err != nil {
			fmt.Fprintf(r.opts.Log, "agent runner: no playbook commands: %v\n", err)
			return nil
		}
		r.playbookLoaded = true
		for _, cmd := range list {
			cmd.Source, cmd.Layer = PlaybookCommandSource, ""
			r.playbookCmds = append(r.playbookCmds, cmd)
		}
	}
	return r.playbookCmds
}

// withPlaybookCommands is list, the commands a lead's harness offers, with
// rec's playbook commands added; a harness command of the same name gives
// way, since the runner expands that name itself. A worker offers no
// commands, so its list is returned as it is.
func (r *Runner) withPlaybookCommands(rec Record, list []wslive.SlashCommand) []wslive.SlashCommand {
	if !rec.Lead {
		return list
	}
	cmds := r.playbookCommands(rec.Backend)
	if len(cmds) == 0 {
		return list
	}
	// A list a snapshot kept may hold an earlier runner's playbook
	// commands; they are replaced whole.
	out := slices.DeleteFunc(slices.Clone(list), func(c wslive.SlashCommand) bool {
		return c.Source == PlaybookCommandSource || slices.ContainsFunc(cmds, func(p wslive.SlashCommand) bool { return p.Name == c.Name })
	})
	return append(out, cmds...)
}

// playbookTurn is the turn text that runs cmd when it is one of the lead's
// playbook commands: the playbook's body (Options.ExpandCommand) under a
// line naming the command and its arguments. A body that cannot be
// rendered becomes a pointer to the read that gets it. ok is false for any
// other command, which runs as the harness's own.
func (c *conv) playbookTurn(cmd wsinbox.Command) (text string, ok bool) {
	rec := c.record()
	if !rec.Lead || !slices.ContainsFunc(c.r.playbookCommands(rec.Backend), func(p wslive.SlashCommand) bool { return p.Name == cmd.Name }) {
		return "", false
	}
	body := ""
	if expand := c.r.opts.ExpandCommand; expand != nil {
		rendered, err := expand(rec.Backend, rec.workDir(), rec.WSKey, cmd.Name)
		if err != nil {
			fmt.Fprintf(c.r.opts.Log, "agent %s: /%s: playbook not rendered, the turn points at it: %v\n", rec.ReplyID, cmd.Name, err)
		} else {
			body = strings.TrimSpace(rendered)
		}
	}
	return playbookCommandTurn(cmd, body), true
}

// playbookCommandTurn frames body, the rendered playbook of cmd, as the
// turn that runs it; an empty body points at the playbook.read that
// returns it instead.
func playbookCommandTurn(cmd wsinbox.Command, body string) string {
	head := fmt.Sprintf("The user ran the `/%s` command with no arguments.", cmd.Name)
	if args := strings.TrimSpace(cmd.Args); args != "" {
		head = fmt.Sprintf("The user ran the `/%s` command with these arguments:\n\n%s", cmd.Name, args)
	}
	if body == "" {
		return head + fmt.Sprintf("\n\nRead the `%s` playbook with the ws `playbook.read` tool and run it.", cmd.Name)
	}
	return head + fmt.Sprintf("\n\nRun the `%s` playbook below; it is the whole playbook, so it needs no `playbook.read`.\n\n<playbook name=%q>\n%s\n</playbook>", cmd.Name, cmd.Name, body)
}
