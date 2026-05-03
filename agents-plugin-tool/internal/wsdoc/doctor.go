package wsdoc

import (
	"fmt"
	"os"
	"path/filepath"
)

type DoctorReport struct {
	OK    bool
	Lines []string
}

func Doctor(root string) DoctorReport {
	root = filepath.Clean(root)
	checks := []struct {
		label string
		path  string
		dir   bool
	}{
		{"repo root", root, true},
		{"ai-docs", filepath.Join(root, "ai-docs"), true},
		{"agents-plugin", filepath.Join(root, "agents-plugin"), true},
		{"claude-plugin", filepath.Join(root, "claude-plugin"), true},
		{"project index", filepath.Join(root, "ai-docs", "_index.md"), false},
	}

	report := DoctorReport{OK: true}
	for _, check := range checks {
		info, err := os.Stat(check.path)
		if err != nil {
			report.OK = false
			report.Lines = append(report.Lines, fmt.Sprintf("missing %s: %s", check.label, check.path))
			continue
		}
		if check.dir && !info.IsDir() {
			report.OK = false
			report.Lines = append(report.Lines, fmt.Sprintf("not a directory %s: %s", check.label, check.path))
			continue
		}
		if !check.dir && info.IsDir() {
			report.OK = false
			report.Lines = append(report.Lines, fmt.Sprintf("not a file %s: %s", check.label, check.path))
			continue
		}
		report.Lines = append(report.Lines, fmt.Sprintf("ok %s: %s", check.label, check.path))
	}
	return report
}
