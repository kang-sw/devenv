package mcp

import (
	"fmt"
	"io"
	"strings"

	"github.com/kang-sw/devenv/internal/wslive"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// HostedRender is what a hosted lead's backend registration
// (wsagent.BackendRegistration) says about text the daemon renders for it:
// Harness, the harness its workflow manual renders under ("" for none), and
// PlaybookCommands, whether its sessions run without ws's plugin skills and
// reach ws only as the daemon's own server.
type HostedRender struct {
	Harness          string
	PlaybookCommands bool
}

// hostedRenderServer is a throwaway server that renders for the hosted lead
// whose key is key as that lead's own connection would: env is the
// environment its ws-mcp serves under (the selected package's resource tree
// and policy) and root its project root. It serves no harness process, so
// it never adopts or validates a hosted session itself; release returns it.
func hostedRenderServer(env []string, root string, render HostedRender, key string) (*Server, func(), error) {
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(name) {
		case envHostedSession, envHostedSessionKey, envHostedSessionRow, envHarnessPID:
			continue
		}
		kept = append(kept, kv)
	}
	s := NewServerWithOptions(ServerOptions{Root: root, Env: kept, Harness: render.Harness, Diag: io.Discard})
	// The harness is declared here, not detected: a name the detection list
	// does not know (agy) still selects the manual's overlay. leadRender
	// renders as the daemon-hosted connection, which lists the agent tools.
	s.descriptor = connectionDescriptor{manualHarness: render.Harness, playbookCommands: render.PlaybookCommands, leadRender: true}
	s.descriptorResolved = true
	// A render may start the session-start tracker check (a background
	// fetch); the server is released once that settles, without holding up
	// the caller.
	release := func() { go func() { s.tracker.checks.Wait(); s.Close() }() }
	if _, ok := s.sessions.lookup(key); !ok {
		release()
		return nil, nil, fmt.Errorf("session key %q has no record", key)
	}
	return s, release, nil
}

// RenderLeadManual renders the workflow manual of the hosted lead whose key
// is key, as workflow_manual returns it on that session's own connection
// (hostedRenderServer), without the read-full notice that tool result opens
// with. The daemon appends it to the lead's system prompt at each launch,
// so the session reads the manual from its first turn.
func RenderLeadManual(env []string, root string, render HostedRender, key string) (string, error) {
	s, release, err := hostedRenderServer(env, root, render, key)
	if err != nil {
		return "", fmt.Errorf("workflow manual: %w", err)
	}
	defer release()
	resp := s.handleWorkflowManual(nil, map[string]any{"session_key": key})
	text := ""
	if result, ok := resp.Result.(map[string]any); ok {
		if content, ok := result["content"].([]map[string]string); ok && len(content) > 0 {
			text = content[0]["text"]
		}
	}
	if responseIsError(resp) {
		return "", fmt.Errorf("workflow manual: %s", text)
	}
	return strings.TrimSpace(strings.TrimPrefix(text, workflowManualReadFullNotice)), nil
}

// RenderLeadPlaybook renders playbook name for the hosted lead whose key is
// key, as playbook.read returns it on that session's own connection
// (hostedRenderServer). The daemon sends it as the turn of the dashboard
// command that runs the playbook, so the session needs no read of its own.
func RenderLeadPlaybook(env []string, root string, render HostedRender, key, name string) (string, error) {
	s, release, err := hostedRenderServer(env, root, render, key)
	if err != nil {
		return "", fmt.Errorf("playbook %s: %w", name, err)
	}
	defer release()
	text, err := s.readPlaybook(name, key, nil)
	if err != nil {
		return "", fmt.Errorf("playbook %s: %w", name, err)
	}
	return strings.TrimSpace(text), nil
}

// HostedPlaybookCommands is the dashboard command list of a hosted lead
// without ws's plugin skills, for the package env's ws-mcp serves: one
// command per user-invocable lead skill of the package (a skill shim whose
// preload block reads its own playbook, wsrsrc.PreloadCommands), described
// as the skill is. The workflow manual's own skill is left out: the lead's
// prompt already carries the manual.
func HostedPlaybookCommands(env []string) ([]wslive.SlashCommand, error) {
	namespace := ""
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && strings.EqualFold(name, envNamespace) {
			namespace = strings.TrimSpace(value)
		}
	}
	skills, err := wsrsrc.PackageSkills(namespace)
	if err != nil {
		return nil, err
	}
	found, err := wsrsrc.PreloadCommands(skills)
	if err != nil {
		return nil, err
	}
	out := make([]wslive.SlashCommand, 0, len(found))
	for _, cmd := range found {
		if cmd.Name == workflowManualPlaybookName {
			continue
		}
		out = append(out, wslive.SlashCommand{Name: cmd.Name, Description: cmd.Description})
	}
	return out, nil
}
