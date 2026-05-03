package wsdoc

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"
)

//go:embed conventions/*.md
var conventionFS embed.FS

func ReadConvention(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("convention document name is required")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") || strings.Contains(name, string(filepath.Separator)) {
		return "", fmt.Errorf("convention document name must be a bare filename or stem")
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	data, err := conventionFS.ReadFile(filepath.ToSlash(filepath.Join("conventions", name)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
