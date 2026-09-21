package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsgit"
)

func (s *Server) handleResolveImplBranch(id json.RawMessage, args map[string]any) response {
	key, err := sessionStateKey("git.resolve_impl_branch", args)
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	record, ok := s.sessions.readState(key)
	if !ok {
		return toolTextResponse(id, "", fmt.Errorf("session key not found: %s", key))
	}
	stem, stemOK := args["ticket_stem"].(string)
	base, baseOK := args["base"].(string)
	stem, base = strings.TrimSpace(stem), strings.TrimSpace(base)
	if !stemOK || stem == "" || !baseOK || !validObservedBranch(base) || base == "HEAD" {
		return toolTextResponse(id, "", fmt.Errorf("ticket_stem and base must be nonempty strings; base must name a branch, not detached HEAD"))
	}
	format, err := parseProceedFormat(args["format"])
	if err != nil {
		return toolTextResponse(id, "", err)
	}
	branch := implTicketBranch(base, stem)
	for _, ref := range []string{base, branch} {
		if _, err := (wsgit.ExecRunner{}).RunGit(context.Background(), record.Root, "check-ref-format", "refs/heads/"+ref); err != nil {
			return toolTextResponse(id, "", fmt.Errorf("invalid branch name %q: %w", ref, err))
		}
	}
	mergeRoot := implementMergeRootFor(base)
	if format == "json" {
		return toolJSONResponse(id, map[string]any{"branch": branch, "merge_root": mergeRoot, "ticket_stem": stem}, nil)
	}
	return toolTextResponse(id, fmt.Sprintf("branch: %s\nmerge_root: %s\n", branch, firstNonEmpty(mergeRoot, "(legacy: requires merge_target policy)")), nil)
}
