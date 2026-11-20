package wsagent

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
)

const ambientBlockName = "ws-ambient"

// WorkflowLanguageInstruction is the canonical workflow.lang instruction used
// by both rendered workflow text and hosted-lead turn ambient.
func WorkflowLanguageInstruction(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return ""
	}
	return "Respond to the user in " + lang +
		". Write ticket frontmatter titles in that language. Keep all internal reasoning, subagent prompts, code comments, and other AI-authored artifacts in English."
}

func ambientBlock(instruction string) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return ""
	}
	return "<" + ambientBlockName + ">\n" + instruction + "\n</" + ambientBlockName + ">"
}

// resolveAmbient resolves one conversation's ambient block outside its send
// path. The key store is an optional session-layer reader; older/test stores
// without that capability still resolve global, repo, and builtin values.
func (c *conv) resolveAmbient() (string, error) {
	if !c.rec.Lead {
		return "", nil
	}
	cfg := c.r.opts.Config
	cfg.RepoRoot = c.rec.Root
	if roots, ok := c.r.opts.Keys.(interface{ Root(string) (string, bool) }); ok {
		if root, found := roots.Root(c.rec.WSKey); found {
			cfg.RepoRoot = root
		}
	}
	reader, _ := c.r.opts.Keys.(wsconfig.SessionReader)
	resolver := wsconfig.NewResolver(cfg, nil, reader, nil)
	resolved, err := resolver.Get(c.rec.WSKey, wsconfig.ItemWorkflowLang)
	if err != nil {
		return "", fmt.Errorf("resolve workflow.lang ambient: %w", err)
	}
	return ambientBlock(WorkflowLanguageInstruction(resolved.Value)), nil
}

// refreshAmbient replaces the loop-owned cache after creation/restoration or
// a successful config.tune. A failed refresh keeps the last known-good block.
func (c *conv) refreshAmbient() {
	block, err := c.resolveAmbient()
	if err != nil {
		fmt.Fprintf(c.r.opts.Log, "agent %s: %v\n", c.rec.ReplyID, err)
		return
	}
	c.ambient = block
}

// turnInput composes model-only turn input. compactInput consumes and prefixes
// its one-shot note first; ambient stays last and is never recorded as user
// text by the dashboard/history paths.
func (c *conv) turnInput(text string) string {
	text = c.compactInput(text)
	if c.ambient == "" {
		return text
	}
	if strings.TrimSpace(text) == "" {
		return c.ambient
	}
	return text + "\n\n" + c.ambient
}

// RefreshAmbient synchronously republishes every open hosted lead's rendered
// ambient block. config.tune calls it only after a successful workflow.lang
// set/reset, so the next turn observes the new value without polling files.
func (r *Runner) RefreshAmbient() {
	r.mu.Lock()
	convs := make([]*conv, 0, len(r.convs))
	for _, c := range r.convs {
		convs = append(convs, c)
	}
	r.mu.Unlock()
	for _, c := range convs {
		c.do(func() { c.refreshAmbient() })
	}
}
