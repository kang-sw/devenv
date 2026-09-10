package wsstore

import (
	"fmt"
	"os"
	"strings"
)

type RuntimeStateSource string

const (
	RuntimeSourceExecJobJSON RuntimeStateSource = "exec/state.json"
)

type RuntimeFieldStorage string

const (
	RuntimeFieldSQLiteMetadata    RuntimeFieldStorage = "sqlite_metadata"
	RuntimeFieldFileBackedPayload RuntimeFieldStorage = "file_backed_payload"
)

type RuntimeWriteAuthority string

const (
	RuntimeAuthoritySQLite RuntimeWriteAuthority = "sqlite"
	RuntimeAuthorityFile   RuntimeWriteAuthority = "file_backed_payload"
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
	out = append(out, sqliteFields(RuntimeSourceExecJobJSON,
		"schema_version", "exec_key", "status", "root", "working_dir", "argv", "command", "shell", "env", "stdin_present", "stdin_bytes", "pid", "started_at", "updated_at", "completed_at", "exit_code", "error", "cancel_requested", "stdout_bytes", "stderr_bytes", "combined_bytes",
	)...)
	out = append(out, fileFields(RuntimeSourceExecJobJSON, "stdout", "stderr", "combined")...)
	return out
}()
