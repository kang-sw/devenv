package wsstore

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

type RuntimeStateSource string

const (
	RuntimeSourceAgentJSON        RuntimeStateSource = "agent.json"
	RuntimeSourceAgentCurrentJSON RuntimeStateSource = "current/state.json"
	RuntimeSourceExecJobJSON      RuntimeStateSource = "exec/state.json"
)

type RuntimeFieldStorage string

const (
	RuntimeFieldSQLiteMetadata      RuntimeFieldStorage = "sqlite_metadata"
	RuntimeFieldFileBackedPayload   RuntimeFieldStorage = "file_backed_payload"
	RuntimeFieldTemporaryCompatOnly RuntimeFieldStorage = "temporary_compatibility_only"
)

type RuntimeWriteAuthority string

const (
	RuntimeAuthoritySQLite RuntimeWriteAuthority = "sqlite"
	RuntimeAuthorityFile   RuntimeWriteAuthority = "file_backed_payload"
	RuntimeAuthorityNone   RuntimeWriteAuthority = "none"
)

type RuntimeFieldClassification struct {
	Source         RuntimeStateSource
	Field          string
	Storage        RuntimeFieldStorage
	WriteAuthority RuntimeWriteAuthority
	Note           string
}

func RuntimeMetadataInventory() []RuntimeFieldClassification {
	inventory := append([]RuntimeFieldClassification(nil), runtimeMetadataInventory...)
	return inventory
}

func RuntimeField(source RuntimeStateSource, field string) (RuntimeFieldClassification, bool) {
	for _, item := range runtimeMetadataInventory {
		if item.Source == source && item.Field == field {
			return item, true
		}
	}
	return RuntimeFieldClassification{}, false
}

func AgentInternalKey(publicName string) (string, error) {
	name := strings.TrimSpace(publicName)
	if name == "" {
		return "", errors.New("agent public name is required")
	}
	return "name:" + url.QueryEscape(name), nil
}

type PayloadConsistency string

const (
	PayloadConsistencyPresent        PayloadConsistency = "present"
	PayloadConsistencyNoPath         PayloadConsistency = "no_path"
	PayloadConsistencyMissingPayload PayloadConsistency = "missing_file_backed_payload_recoverable"
)

func ClassifyFileBackedPayload(path string) PayloadConsistency {
	if strings.TrimSpace(path) == "" {
		return PayloadConsistencyNoPath
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return PayloadConsistencyMissingPayload
		}
		return PayloadConsistencyMissingPayload
	}
	return PayloadConsistencyPresent
}

func ValidateRuntimeMetadataInventory() error {
	seen := map[string]bool{}
	for _, item := range runtimeMetadataInventory {
		if item.Source == "" || item.Field == "" || item.Storage == "" || item.WriteAuthority == "" {
			return fmt.Errorf("incomplete runtime metadata classification: %#v", item)
		}
		key := string(item.Source) + "\x00" + item.Field
		if seen[key] {
			return fmt.Errorf("duplicate runtime metadata classification for %s %s", item.Source, item.Field)
		}
		seen[key] = true
		if item.Source == RuntimeSourceAgentJSON && item.Field != "agent_json_compatibility" && item.WriteAuthority != RuntimeAuthoritySQLite {
			return fmt.Errorf("agent.json metadata field %s is not sqlite-authoritative", item.Field)
		}
	}
	return nil
}

func sqliteFields(source RuntimeStateSource, fields ...string) []RuntimeFieldClassification {
	out := make([]RuntimeFieldClassification, 0, len(fields))
	for _, field := range fields {
		out = append(out, RuntimeFieldClassification{Source: source, Field: field, Storage: RuntimeFieldSQLiteMetadata, WriteAuthority: RuntimeAuthoritySQLite})
	}
	return out
}

func fileFields(source RuntimeStateSource, fields ...string) []RuntimeFieldClassification {
	out := make([]RuntimeFieldClassification, 0, len(fields))
	for _, field := range fields {
		out = append(out, RuntimeFieldClassification{Source: source, Field: field, Storage: RuntimeFieldFileBackedPayload, WriteAuthority: RuntimeAuthorityFile})
	}
	return out
}

var runtimeMetadataInventory = func() []RuntimeFieldClassification {
	var out []RuntimeFieldClassification
	out = append(out, sqliteFields(RuntimeSourceAgentJSON,
		"schema_version", "name", "backend", "harness", "tier", "model", "effort", "session_id", "status",
		"created_at", "last_seen_at", "last_call_at", "prompt_refs", "system_prompt_path", "last_output_path", "child_actor_id", "child_actor_authority", "capabilities", "ephemeral",
	)...)
	out = append(out, RuntimeFieldClassification{Source: RuntimeSourceAgentJSON, Field: "agent_json_compatibility", Storage: RuntimeFieldTemporaryCompatOnly, WriteAuthority: RuntimeAuthorityNone, Note: "bounded read-only import, tombstone, or diagnostic bridge; not metadata write authority"})

	out = append(out, sqliteFields(RuntimeSourceAgentCurrentJSON,
		"schema_version", "agent_name", "call_seq", "execution_id", "status", "pid", "started_at", "updated_at", "finished_at", "prompt_path", "stdout_path", "stderr_path", "exit_code", "session_id", "error", "cleanup_needed", "cancel_pid",
	)...)

	out = append(out, sqliteFields(RuntimeSourceExecJobJSON,
		"schema_version", "exec_key", "status", "root", "working_dir", "argv", "command", "shell", "env", "stdin_present", "stdin_bytes", "pid", "started_at", "updated_at", "completed_at", "exit_code", "error", "cancel_requested", "stdout_bytes", "stderr_bytes", "combined_bytes",
	)...)
	out = append(out, fileFields(RuntimeSourceExecJobJSON, "stdout", "stderr", "combined")...)
	return out
}()
