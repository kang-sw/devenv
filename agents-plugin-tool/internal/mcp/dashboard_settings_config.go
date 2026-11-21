package mcp

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsconfigservice"
	"github.com/kang-sw/devenv/internal/wsdashboard"
	"github.com/kang-sw/devenv/internal/wspackage"
)

func DashboardConfigCatalog(packageName string) (*wsconfigservice.Catalog, error) {
	pkg, err := wspackage.Open(packageName)
	if err != nil {
		return nil, err
	}
	rsrc, err := fs.Sub(pkg, "rsrc")
	if err != nil {
		return nil, err
	}
	return dashboardConfigCatalog(pkg, rsrc)
}

func dashboardConfigCatalog(pkg, rsrc fs.FS) (*wsconfigservice.Catalog, error) {
	defaults := builtinConfigAndPromptDefaults()
	rules := make([]wsconfigservice.Rule, 0, len(configRegistry)+40)
	add := func(entry configKeyEntry) {
		rule := wsconfigservice.Rule{
			Key: entry.Key, Type: "string", Default: defaults[entry.Key],
			GlobalOnly: entry.GlobalOnly(), Structured: entry.Structured,
			Compaction: isCompactionKey(entry.Key), Preference: wsconfig.ValidPreferenceKey(entry.Key),
			Enum: fieldEnum(entry.ValueFields, "value"),
			Normalize: func(value string) (string, error) {
				if strings.HasPrefix(entry.Key, "prompt.") {
					if strings.TrimSpace(value) == "" {
						return "", fmt.Errorf("config.tune: prompt must be non-empty")
					}
					return value, nil
				}
				return normalizeTuneScalar(entry, value, true)
			},
		}
		if entry.HarnessApplicable {
			rule.Harnesses = fieldEnum(entry.SelectorFields, "harness")
		}
		if entry.Declared != nil {
			key := entry.Declared
			rule.Default, rule.Type = key.Default, key.Type
			rule.Minimum, rule.Maximum = key.Minimum, key.Maximum
			rule.Decode = key.parse
			rule.NoRepo, rule.NoSession = dashboardAdapterReadLimits(key.Key)
		}
		if rule.Compaction {
			rule.LegacyKey = func(harness string) string { return wsagent.BuiltinCompaction(harness).LegacyKey(entry.Key) }
		}
		if entry.Structured {
			rule.Type = "object"
			family := worktreeProfileFamily
			if strings.HasPrefix(entry.Key, wsagent.ProfilePrefix) {
				family = registeredAgentProfileFamily()
			}
			rule.ValidateName = family.validateName
			rule.NormalizeObject = func(value any, scope wsconfig.Scope) (json.RawMessage, error) {
				if _, ok := value.(map[string]any); !ok {
					return nil, wsconfigservice.ErrInvalid
				}
				raw, err := json.Marshal(value)
				if err != nil {
					return nil, err
				}
				parsed, err := family.parse(raw, scope)
				if err != nil {
					return nil, err
				}
				return json.Marshal(parsed)
			}
		}
		if entry.Key == "agents.tier" {
			rule.Type = "tier"
		}
		rules = append(rules, rule)
	}
	for _, entry := range configRegistry {
		add(entry)
	}
	for _, key := range append([]declaredConfigKey{worktreeNudgeStaleDaysKey, worktreeNudgePreservedGBKey}, gitMergeSizeKeys...) {
		add(key.entry())
	}
	// The pool is visible in config.show and resolved by worktree tools.
	// It has no config.tune writer, so the settings sheet can inspect it.
	rules = append(rules, wsconfigservice.Rule{Key: wsconfig.ItemWorktreePool, Type: "string", Default: defaults[wsconfig.ItemWorktreePool], ReadOnly: true})
	add(preferenceKnobEntry("preference.*"))
	add(worktreeProfileKnobEntry("*"))
	add(agentProfileKnobEntry("*"))
	points, seeds, err := scanOverridePointsAndSeeds(rsrc)
	if err != nil {
		return nil, err
	}
	for _, point := range points {
		add(promptKnobEntry(point.PointId))
		if seed, ok := seeds[point.PointId]; ok {
			rules[len(rules)-1].Seed = &seed
		}
	}
	namespaces := make(map[string]string)
	for _, name := range []string{wspackage.ConfigManifestFile, wspackage.AgYConfigManifestFile, wspackage.DashboardConfigManifestFile, wspackage.PiConfigManifestFile} {
		raw, err := fs.ReadFile(pkg, name)
		if err != nil {
			return nil, err
		}
		namespace, keys, err := parseConfigManifestBytes(name, raw, builtinConfigKeySet(), namespaces)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		namespaces[namespace] = name
		for _, key := range keys {
			add(key.entry())
		}
	}
	return wsconfigservice.NewCatalog(rules)
}

func DashboardConfigBackend(packageName string, opts wsconfig.Options, src *DashboardSessionSource, ambient func()) (*wsdashboard.ConfigBackend, error) {
	catalog, err := DashboardConfigCatalog(packageName)
	if err != nil {
		return nil, err
	}
	structured := builtinWorktreeProfiles()
	for key, value := range wsagent.BuiltinProfiles() {
		structured[key] = value
	}
	return &wsdashboard.ConfigBackend{
		Catalog: catalog, Options: opts, Structured: structured,
		Session:            src.settingsSession,
		IsLead:             src.settingsLead,
		OrchestrateWarning: orchestrateWhitelistWarning,
		Changed: func(key string) {
			notifyCompactionTune(key)
			if key == wsconfig.ItemWorkflowLang && ambient != nil {
				ambient()
			}
		},
	}, nil
}

// settingsSession batches the lineage read without touching session activity.
// Its writer binds the secret key in the daemon and never serializes it.
func (src *DashboardSessionSource) settingsSession(reg wsdashboard.Registration) (map[string]string, wsconfig.SessionWriter, string, bool) {
	d, err := src.store.dirs()
	if err != nil {
		return nil, nil, "", false
	}
	keys, err := src.keysFor(d, reg.ReplyID, false)
	if err != nil {
		return nil, nil, "", false
	}
	src.store.mu.Lock()
	defer src.store.mu.Unlock()
	for _, key := range keys {
		record, ok := src.store.readRecord(d, key, partKey)
		if !ok || record.Root != reg.Project {
			continue
		}
		values := make(map[string]string)
		src.store.walkLineage(d, key, record, func(r sessionRecord) bool {
			for k, v := range r.Overrides {
				if _, seen := values[k]; !seen {
					values[k] = v
				}
			}
			return false
		})
		return values, sessionConfigAdapter{s: src.store}, key, true
	}
	return nil, nil, "", false
}

// Startup-only adapter readers do not have a session key or worktree.
// These limits describe the effective adapter behavior, while MCP keeps
// accepting its historical config.tune arguments.
func dashboardAdapterReadLimits(key string) (noRepo, noSession bool) {
	switch key {
	case "claude.status_band_enabled":
		return false, false
	case "pi.runaway_threshold", "pi.settle_delay_ms", "pi.compaction_advisory_percent", "pi.compaction_hard_percent",
		"pi.context_window_override", "pi.compaction_dialog_budget_bytes", "pi.compaction_reread_budget_bytes":
		return false, false
	case "agy.model_wait_timeout_seconds", "agy.model_wait_retries":
		return false, true
	case DashboardPortKey, "dashboard.pi_spawn_enabled":
		return true, true
	}
	if strings.HasPrefix(key, "claude.") || strings.HasPrefix(key, "pi.") {
		return true, true
	}
	return false, false
}

func (src *DashboardSessionSource) settingsLead(reg wsdashboard.Registration) bool {
	d, err := src.store.dirs()
	if err != nil {
		return false
	}
	keys, err := src.keysFor(d, reg.ReplyID, false)
	if err != nil {
		return false
	}
	src.store.mu.Lock()
	defer src.store.mu.Unlock()
	for _, key := range keys {
		record, ok := src.store.readRecord(d, key, partKey)
		if ok && record.Root == reg.Project && record.Parent == "" && record.Scope == string(roleLead) {
			return true
		}
	}
	return false
}
