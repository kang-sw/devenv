package wsxmlcmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Problems collects every problem one batch has, so a refusal can list them
// all; a batch with any problem applies nothing.
type Problems struct {
	List []string
}

// Add records one problem.
func (p *Problems) Add(format string, args ...any) {
	p.List = append(p.List, fmt.Sprintf(format, args...))
}

// NonEmpty reports whether s holds non-blank text, recording a problem
// under label when not.
func NonEmpty(p *Problems, label, s string) bool {
	if strings.TrimSpace(s) == "" {
		p.Add("%s: must be non-empty", label)
		return false
	}
	return true
}

// RequiredAttr returns a command's non-blank attribute.
func RequiredAttr(p *Problems, cmd Command, name string) (string, bool) {
	v, present := cmd.Attr(name)
	if !present {
		p.Add("%s: required", cmd.At(name))
		return "", false
	}
	return v, NonEmpty(p, cmd.At(name), v)
}

// EnumAttr returns a command's required attribute when it is one of values.
func EnumAttr(p *Problems, cmd Command, name string, values []string) (string, bool) {
	v, ok := RequiredAttr(p, cmd, name)
	if !ok {
		return "", false
	}
	if !slices.Contains(values, v) {
		p.Add("%s: must be one of %s; got %q", cmd.At(name), strings.Join(values, ", "), v)
		return "", false
	}
	return v, true
}

// IntAttr returns a command's positive whole-number attribute; required
// records a problem when it is absent.
func IntAttr(p *Problems, cmd Command, name string, required bool) (int, bool) {
	v, present := cmd.Attr(name)
	if !present {
		if required {
			p.Add("%s: required", cmd.At(name))
		}
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		p.Add("%s: must be a positive whole number; got %q", cmd.At(name), v)
		return 0, false
	}
	return n, true
}

// BoolAttr returns a command's "true"/"false" attribute, false when absent.
func BoolAttr(p *Problems, cmd Command, name string) (bool, bool) {
	v, present := cmd.Attr(name)
	if !present {
		return false, true
	}
	switch v {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	p.Add("%s: must be \"true\" or \"false\"; got %q", cmd.At(name), v)
	return false, false
}
