package mcp

import (
	"fmt"

	"github.com/kang-sw/devenv/internal/wsagent"
)

// toolAliasTable spells ws's canonical (dotted) tool names the way one
// connection's host accepts them. A nil table is the identity.
type toolAliasTable struct {
	toAlias     map[string]string
	toCanonical map[string]string
}

// buildToolAliasTable spells every name under scheme (wsagent.ApplyToolNameScheme)
// and refuses a table that is not one-to-one: two names with one alias, or an
// alias that is another tool's canonical name, would make a call ambiguous.
// The identity scheme returns nil.
func buildToolAliasTable(scheme string, names []string) (*toolAliasTable, error) {
	if scheme == "" {
		return nil, nil
	}
	canonical := make(map[string]bool, len(names))
	for _, name := range names {
		canonical[name] = true
	}
	table := &toolAliasTable{toAlias: make(map[string]string, len(names)), toCanonical: make(map[string]string, len(names))}
	for _, name := range names {
		alias, ok := wsagent.ApplyToolNameScheme(scheme, name)
		if !ok {
			return nil, fmt.Errorf("unknown tool-name scheme %q", scheme)
		}
		if other, taken := table.toCanonical[alias]; taken && other != name {
			return nil, fmt.Errorf("tool-name scheme %q gives %q and %q one alias %q", scheme, other, name, alias)
		}
		if canonical[alias] && alias != name {
			return nil, fmt.Errorf("tool-name scheme %q spells %q as %q, the canonical name of another tool", scheme, name, alias)
		}
		table.toAlias[name] = alias
		table.toCanonical[alias] = name
	}
	return table, nil
}

// advertised is the name a connection's tool list shows for canonical name.
func (t *toolAliasTable) advertised(name string) string {
	if t != nil {
		if alias, ok := t.toAlias[name]; ok {
			return alias
		}
	}
	return name
}

// canonical maps an advertised alias back to its canonical name; any other
// name, a canonical one included, is returned as it is.
func (t *toolAliasTable) canonical(name string) string {
	if t != nil {
		if canonical, ok := t.toCanonical[name]; ok {
			return canonical
		}
	}
	return name
}

// connectionDescriptor is what a hosted lead's backend registration says
// about the connection: its tool-name aliases and the harness its workflow
// manual renders under. The zero value is an ordinary connection.
type connectionDescriptor struct {
	aliases       *toolAliasTable
	manualHarness string
}

// connectionDescriptor reads the descriptor of the backend registration the
// connection's hosted lead record names (wsagent.Record.Backend), through the
// runner that hosts it. It is resolved once; a hosted connection whose record
// is not readable yet is asked again at its next call. A connection that is
// not a hosted lead's, or has no runner, is the zero descriptor.
func (s *Server) connectionDescriptor() connectionDescriptor {
	s.descriptorMu.Lock()
	defer s.descriptorMu.Unlock()
	if s.descriptorResolved {
		return s.descriptor
	}
	hosted := s.hostedSessionKey()
	if hosted == "" {
		s.descriptorResolved = true
		return s.descriptor
	}
	rec, ok := s.callerRecord(hosted)
	if !ok {
		return connectionDescriptor{}
	}
	s.descriptorResolved = true
	registrations := s.agentBackends()
	if len(registrations) == 0 {
		return s.descriptor
	}
	reg, ok := registrations[0][rec.Backend]
	if !ok {
		return s.descriptor
	}
	s.descriptor.manualHarness = reg.ManualHarness
	names := make([]string, 0, len(tools()))
	for _, tool := range tools() {
		if name, _ := tool["name"].(string); name != "" {
			names = append(names, name)
		}
	}
	table, err := buildToolAliasTable(reg.ToolNameScheme, names)
	if err != nil {
		// Dotted names still work for a host that takes them.
		if s.diag != nil {
			fmt.Fprintf(s.diag, "ws-mcp: tool aliases disabled for this connection: %v\n", err)
		}
	}
	s.descriptor.aliases = table
	return s.descriptor
}

// playbookHarness is the harness a playbook render selects its overlay
// under: the connection's detected harness, else the one its backend
// registration declares for a hosted lead's manual.
func (s *Server) playbookHarness() string {
	if harness := s.currentHarness(); harness != "" {
		return harness
	}
	return s.connectionDescriptor().manualHarness
}
