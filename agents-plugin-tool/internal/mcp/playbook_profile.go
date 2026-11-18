package mcp

import (
	"fmt"
	"strings"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

// Profile selection (workflow.profile) for playbook loads. Only a top-level
// lead key selects a profile: a parented key (sub-lead, worker) and a keyless
// or unknown key always load the base files, so a sub-lead reads the full
// manual. The profile is resolved once per call and handed to every wsrsrc
// load of that call, declared-variable collection included.

// resolvedPlaybookProfile reads workflow.profile for key through resolver,
// which must be built with builtinConfigDefaults. An unrecognized stored value
// selects the base files rather than failing the load.
func (s *Server) resolvedPlaybookProfile(resolver *wsconfig.Resolver, key string) (string, error) {
	rv, _, err := resolveHarnessBucketed(resolver, key, wsconfig.ItemWorkflowProfile, s.currentHarness())
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", wsconfig.ItemWorkflowProfile, err)
	}
	if profile := strings.ToLower(strings.TrimSpace(rv.Value)); wsrsrc.IsProfileName(profile) {
		return profile, nil
	}
	return "", nil
}

// callerPlaybookProfile is the profile a playbook load by key selects: the
// resolved workflow.profile for a top-level lead key, "" otherwise.
func (s *Server) callerPlaybookProfile(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", nil
	}
	entry, found := s.lookupSession(key)
	if !found || !entry.isTopLevelLead() {
		return "", nil
	}
	resolver := s.sessionResolver(key, builtinConfigDefaults())
	return s.resolvedPlaybookProfile(&resolver, key)
}
