package wsagent

// Claude keeps its local-settings store at cwd on Windows: it cannot check
// POSIX directory ownership before reading a canonical repository's store.
func claudeSettingsOwnedByUser(string, bool) bool { return false }
