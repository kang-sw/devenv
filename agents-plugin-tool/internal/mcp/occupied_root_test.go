package mcp

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kang-sw/devenv/internal/wsgit"
)

type occupiedRootRunner func(context.Context, string, ...string) ([]byte, error)

func (r occupiedRootRunner) RunGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	return r(ctx, root, args...)
}

func TestGuardOccupiedRoot(t *testing.T) {
	for _, tc := range []struct {
		name, ref string
		allowed   []string
		blocked   bool
	}{
		{"worker", "refs/heads/impl/develop/worker", nil, true},
		{"rootless", "refs/heads/impl/worker", nil, true},
		{"nested-parent", "refs/heads/impl/goal/develop/topic/worker", nil, true},
		{"own-source", "refs/heads/impl/develop/worker", []string{"impl/develop/worker", "develop"}, false},
		{"own-target", "refs/heads/impl/develop/worker", []string{"feature/topic", "impl/develop/worker"}, false},
		{"different-worker", "refs/heads/impl/develop/worker", []string{"impl/develop/other", "develop"}, true},
		{"not-a-prefix", "refs/heads/topic/impl/develop/worker", nil, false},
		{"develop", "refs/heads/develop", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			runner := occupiedRootRunner(func(ctx context.Context, root string, args ...string) ([]byte, error) {
				calls++
				if root != "root" || !reflect.DeepEqual(args, []string{"symbolic-ref", "--quiet", "HEAD"}) {
					t.Fatalf("unexpected inspection: %q %v", root, args)
				}
				return []byte(tc.ref + "\n"), nil
			})
			err := guardOccupiedRoot(context.Background(), runner, "root", tc.allowed...)
			var occupied *occupiedRootError
			if errors.As(err, &occupied) != tc.blocked || (err != nil && !tc.blocked) || calls != 1 {
				t.Fatalf("blocked=%t calls=%d err=%v", tc.blocked, calls, err)
			}
			if tc.blocked {
				branch := strings.TrimPrefix(tc.ref, "refs/heads/")
				if occupied.branch != branch || !strings.Contains(err.Error(), branch) || !strings.Contains(err.Error(), `profile: "housekeeping"`) || !strings.Contains(err.Error(), "returned session key") {
					t.Fatalf("missing recovery guidance: %v", err)
				}
			}
		})
	}
}

func TestGuardOccupiedRootInspection(t *testing.T) {
	root := implRootRepo(t, "impl/develop/worker")
	runGit(t, root, "checkout", "--detach")
	if err := guardOccupiedRoot(context.Background(), wsgit.ExecRunner{}, root); err != nil {
		t.Fatalf("detached HEAD is not an occupied impl checkout: %v", err)
	}
	for _, tc := range []struct {
		name, out string
		err       error
	}{
		{"git-failure", "", fmt.Errorf("cannot read HEAD")},
		{"empty", "", nil},
		{"malformed", "not-a-head-ref", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := occupiedRootRunner(func(context.Context, string, ...string) ([]byte, error) {
				return []byte(tc.out), tc.err
			})
			err := guardOccupiedRoot(context.Background(), runner, root)
			var occupied *occupiedRootError
			if err == nil || errors.As(err, &occupied) {
				t.Fatalf("inspection must fail closed without claiming an occupant: %v", err)
			}
		})
	}
}
