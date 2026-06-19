package mcp

// sessionConfigAdapter adapts *sessionStore to the wsconfig.SessionReader and
// wsconfig.SessionWriter interfaces so the resolver can route session-scope
// config reads and writes through the existing session store without coupling
// wsconfig to the mcp package.
//
// It also implements the optional ListOverrideKeys interface that ScopedShow
// uses to discover session-only keys (keys not present in project or global).
type sessionConfigAdapter struct {
	s *sessionStore
}

// GetOverride implements wsconfig.SessionReader.
func (a sessionConfigAdapter) GetOverride(sessionKey, itemKey string) (string, bool) {
	return a.s.getOverride(sessionKey, itemKey)
}

// SetOverride implements wsconfig.SessionWriter.
func (a sessionConfigAdapter) SetOverride(sessionKey, itemKey, value string) error {
	return a.s.setOverride(sessionKey, itemKey, value)
}

// ListOverrideKeys returns all item keys present in the session record for the
// given session key. This optional method is consumed by wsconfig.ScopedShow
// to enumerate session-only keys that are not visible via the project or global
// config files.
func (a sessionConfigAdapter) ListOverrideKeys(sessionKey string) []string {
	return a.s.listOverrideKeys(sessionKey)
}
