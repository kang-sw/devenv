package wsgit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type commitMergeFixture struct {
	root, ours, incoming string
}

func newCommitMergeFixture(t *testing.T, oursExtra, incomingExtra func(string)) commitMergeFixture {
	t.Helper()
	root := t.TempDir()
	sparseTestInitGit(t, root)
	sparseTestRunGit(t, root, "switch", "-c", "ours")
	sparseTestRunGit(t, root, "config", "commit.gpgsign", "false")
	for path, content := range map[string]string{
		"conflict.txt": "base\n", "incoming.txt": "base incoming\n", "delete.txt": "delete me\n",
		"unrelated.txt": "keep me\n", "files/rename.txt": strings.Repeat("rename source content\n", 20),
		"shape": "file before directory\n",
	} {
		mustWriteGitTestFixture(t, root, path, content)
	}
	sparseTestRunGit(t, root, "add", ".")
	sparseTestRunGit(t, root, "commit", "-m", "base")
	sparseTestRunGit(t, root, "switch", "-c", "incoming")
	mustWriteGitTestFixture(t, root, "conflict.txt", "theirs\n")
	mustWriteGitTestFixture(t, root, "incoming.txt", "incoming version\n")
	mustWriteGitTestFixture(t, root, "added.txt", "incoming addition\n")
	sparseTestRunGit(t, root, "rm", "delete.txt")
	if incomingExtra != nil {
		incomingExtra(root)
	}
	sparseTestRunGit(t, root, "add", ".")
	sparseTestRunGit(t, root, "commit", "-m", "incoming")
	incoming := strings.TrimSpace(string(sparseTestRunGitOutput(t, root, "rev-parse", "HEAD")))
	sparseTestRunGit(t, root, "switch", "ours")
	mustWriteGitTestFixture(t, root, "conflict.txt", "ours\n")
	if oursExtra != nil {
		oursExtra(root)
	}
	sparseTestRunGit(t, root, "add", ".")
	sparseTestRunGit(t, root, "commit", "-m", "ours")
	ours := strings.TrimSpace(string(sparseTestRunGitOutput(t, root, "rev-parse", "HEAD")))
	cmd := exec.Command("git", "-C", root, "-c", "merge.directoryRenames=false", "merge", "--no-ff", "--no-commit", "incoming")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "CONFLICT") {
		t.Fatalf("expected conflicted merge: err=%v, output=%s", err, out)
	}
	mustWriteGitTestFixture(t, root, "conflict.txt", "resolved\n")
	return commitMergeFixture{root: root, ours: ours, incoming: incoming}
}

func (f commitMergeFixture) opts(paths ...string) CommitOptions {
	if len(paths) == 0 {
		paths = []string{"conflict.txt"}
	}
	return CommitOptions{Paths: paths, Title: "fix: conclude merge", AIContext: []string{"Preserve scoped merge content."}, ExpectedBranch: "ours"}
}

func (f commitMergeFixture) mergeFile(t *testing.T) string {
	t.Helper()
	path := strings.TrimSpace(string(sparseTestRunGitOutput(t, f.root, "rev-parse", "--git-path", "MERGE_HEAD")))
	if !filepath.IsAbs(path) {
		path = filepath.Join(f.root, path)
	}
	return path
}

func TestCommitMergeAdmitsExactIncomingContent(t *testing.T) {
	f := newCommitMergeFixture(t, nil, func(root string) {
		mustWriteGitTestFixture(t, root, "space and\nnewline.txt", "unusual incoming name\n")
	})
	mustWriteGitTestFixture(t, f.root, "unrelated.txt", "unstaged and retained\n")
	// The structured record must not depend on a user's cleanup configuration.
	sparseTestRunGit(t, f.root, "config", "commit.cleanup", "strip")
	var verified []string
	client := Client{Verifier: func(root string, paths []string) ([]string, error) {
		verified = append([]string(nil), paths...)
		return []string{"incoming paths verified"}, nil
	}}
	result, err := client.Commit(context.Background(), f.root, f.opts())
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"added.txt", "conflict.txt", "incoming.txt", "space and\nnewline.txt"}
	sort.Strings(verified)
	if !reflect.DeepEqual(verified, wantPaths) {
		t.Fatalf("verified = %q, want %q", verified, wantPaths)
	}
	if !reflect.DeepEqual(result.Advisories, []string{"incoming paths verified"}) {
		t.Fatalf("advisories = %q", result.Advisories)
	}
	parents := strings.Fields(string(sparseTestRunGitOutput(t, f.root, "show", "-s", "--format=%P", "HEAD")))
	if !reflect.DeepEqual(parents, []string{f.ours, f.incoming}) {
		t.Fatalf("parents = %q", parents)
	}
	message := string(sparseTestRunGitOutput(t, f.root, "show", "-s", "--format=%B", "HEAD"))
	if !strings.Contains(message, "## AI Context\n- Preserve scoped merge content.") {
		t.Fatalf("lost structured context: %s", message)
	}
	if _, err := os.Stat(f.mergeFile(t)); !os.IsNotExist(err) {
		t.Fatalf("merge state not cleared: %v", err)
	}
	if content := string(sparseTestRunGitOutput(t, f.root, "show", "HEAD:incoming.txt")); content != "incoming version\n" {
		t.Fatalf("incoming content = %q", content)
	}
	if content := string(sparseTestRunGitOutput(t, f.root, "show", "HEAD:conflict.txt")); content != "resolved\n" {
		t.Fatalf("resolution = %q", content)
	}
	if content := string(sparseTestRunGitOutput(t, f.root, "show", "HEAD:unrelated.txt")); content != "keep me\n" {
		t.Fatalf("unrelated work was committed: %q", content)
	}
	if _, err := os.Stat(filepath.Join(f.root, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("incoming deletion not retained: %v", err)
	}
	if status := string(sparseTestRunGitOutput(t, f.root, "status", "--porcelain")); status != " M unrelated.txt\n" {
		t.Fatalf("dirty work not preserved: %q", status)
	}
}

func TestCommitMergeRefusesUnapprovedStaging(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, commitMergeFixture)
		want   string
	}{
		{"unrelated addition", func(t *testing.T, f commitMergeFixture) {
			mustWriteGitTestFixture(t, f.root, "unrelated-added.txt", "unapproved\n")
			sparseTestRunGit(t, f.root, "add", "unrelated-added.txt")
		}, "unrelated-added.txt"},
		{"unrelated edit", func(t *testing.T, f commitMergeFixture) {
			mustWriteGitTestFixture(t, f.root, "unrelated.txt", "unapproved\n")
			sparseTestRunGit(t, f.root, "add", "unrelated.txt")
		}, "unrelated.txt"},
		{"same path contamination", func(t *testing.T, f commitMergeFixture) {
			mustWriteGitTestFixture(t, f.root, "incoming.txt", "unapproved same path\n")
			sparseTestRunGit(t, f.root, "add", "incoming.txt")
		}, "incoming.txt"},
		{"same path mode contamination", func(t *testing.T, f commitMergeFixture) {
			sparseTestRunGit(t, f.root, "update-index", "--chmod=+x", "incoming.txt")
		}, "incoming.txt"},
		{"contaminated addition", func(t *testing.T, f commitMergeFixture) {
			mustWriteGitTestFixture(t, f.root, "added.txt", "unapproved addition\n")
			sparseTestRunGit(t, f.root, "add", "added.txt")
		}, "added.txt"},
		{"contaminated deletion", func(t *testing.T, f commitMergeFixture) {
			mustWriteGitTestFixture(t, f.root, "delete.txt", "resurrected\n")
			sparseTestRunGit(t, f.root, "add", "delete.txt")
		}, "delete.txt"},
		{"dirty incoming working file", func(t *testing.T, f commitMergeFixture) {
			mustWriteGitTestFixture(t, f.root, "incoming.txt", "not what is staged\n")
		}, "unstaged changes"},
		{"missing merge state", func(t *testing.T, f commitMergeFixture) {
			if err := os.Remove(f.mergeFile(t)); err != nil {
				t.Fatal(err)
			}
		}, "MERGE_HEAD"},
		{"invalid merge state", func(t *testing.T, f commitMergeFixture) {
			if err := os.WriteFile(f.mergeFile(t), []byte("not a commit\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "single valid MERGE_HEAD"},
		{"nonexistent merge object", func(t *testing.T, f commitMergeFixture) {
			if err := os.WriteFile(f.mergeFile(t), []byte(strings.Repeat("a", 40)+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "valid commit"},
		{"multiple merge heads", func(t *testing.T, f commitMergeFixture) {
			if err := os.WriteFile(f.mergeFile(t), []byte(f.incoming+"\n"+f.ours+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, "single valid MERGE_HEAD"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newCommitMergeFixture(t, nil, nil)
			sparseTestRunGit(t, f.root, "add", "conflict.txt")
			test.mutate(t, f)
			indexBefore := string(sparseTestRunGitOutput(t, f.root, "ls-files", "--stage", "-z"))
			mergeBefore, mergeErr := os.ReadFile(f.mergeFile(t))
			_, err := (Client{}).Commit(context.Background(), f.root, f.opts())
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(err.Error(), "existing paths grant") {
				t.Fatalf("error = %v, want %q and explicit recovery", err, test.want)
			}
			if after := strings.TrimSpace(string(sparseTestRunGitOutput(t, f.root, "rev-parse", "HEAD"))); after != f.ours {
				t.Fatalf("refusal moved HEAD: %s", after)
			}
			if after := string(sparseTestRunGitOutput(t, f.root, "ls-files", "--stage", "-z")); after != indexBefore {
				t.Fatal("refusal changed staged content")
			}
			mergeAfter, afterErr := os.ReadFile(f.mergeFile(t))
			if string(mergeBefore) != string(mergeAfter) || os.IsNotExist(mergeErr) != os.IsNotExist(afterErr) {
				t.Fatal("refusal altered merge state")
			}
		})
	}
}

func TestCommitMergeExplicitGrantRecoversCompositeContent(t *testing.T) {
	f := newCommitMergeFixture(t, func(root string) {
		mustWriteGitTestFixture(t, root, "incoming.txt", "our composite choice\n")
	}, nil)
	mustWriteGitTestFixture(t, f.root, "incoming.txt", "resolved composite\n")
	sparseTestRunGit(t, f.root, "add", "incoming.txt")
	if _, err := (Client{}).Commit(context.Background(), f.root, f.opts()); err == nil || !strings.Contains(err.Error(), "incoming.txt") {
		t.Fatalf("composite content admitted: %v", err)
	}
	if _, err := (Client{}).Commit(context.Background(), f.root, f.opts("conflict.txt", "incoming.txt")); err != nil {
		t.Fatalf("explicit grant did not recover: %v", err)
	}
}

func TestCommitMergeRenameAndFileDirectoryBoundaries(t *testing.T) {
	t.Run("pure incoming rename", func(t *testing.T) {
		f := newCommitMergeFixture(t, nil, func(root string) {
			sparseTestRunGit(t, root, "mv", "files/rename.txt", "files/renamed.txt")
		})
		var verified []string
		_, err := (Client{Verifier: func(_ string, paths []string) ([]string, error) {
			verified = paths
			return nil, nil
		}}).Commit(context.Background(), f.root, f.opts())
		if err != nil {
			t.Fatal(err)
		}
		if !containsCommitPath(verified, "files/renamed.txt") || containsCommitPath(verified, "files/rename.txt") {
			t.Fatalf("rename verification = %q", verified)
		}
	})
	t.Run("our rename with incoming edit", func(t *testing.T) {
		f := newCommitMergeFixture(t, func(root string) {
			sparseTestRunGit(t, root, "mv", "files/rename.txt", "files/ours.txt")
		}, func(root string) {
			mustWriteGitTestFixture(t, root, "files/rename.txt", strings.Repeat("rename source content\n", 20)+"incoming edit\n")
		})
		if _, err := (Client{}).Commit(context.Background(), f.root, f.opts()); err == nil || !strings.Contains(err.Error(), "files/ours.txt") {
			t.Fatalf("our rename admitted: %v", err)
		}
		if _, err := (Client{}).Commit(context.Background(), f.root, f.opts("conflict.txt", "files")); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("file directory replacement", func(t *testing.T) {
		f := newCommitMergeFixture(t, nil, func(root string) {
			sparseTestRunGit(t, root, "rm", "shape")
			mustWriteGitTestFixture(t, root, "shape/child.txt", "directory replacement\n")
		})
		if _, err := (Client{}).Commit(context.Background(), f.root, f.opts()); err == nil || !strings.Contains(err.Error(), "file/directory") {
			t.Fatalf("file/directory replacement admitted: %v", err)
		}
		if _, err := (Client{}).Commit(context.Background(), f.root, f.opts("conflict.txt", "shape")); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCommitMergeRefusesAmbiguousOrMissingBase(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		t.Run(fmt.Sprint(ambiguous), func(t *testing.T) {
			f := newCommitMergeFixture(t, nil, nil)
			tree := strings.TrimSpace(string(sparseTestRunGitOutput(t, f.root, "rev-parse", f.ours+"^{tree}")))
			args := []string{"commit-tree", tree, "-m", "other root"}
			if ambiguous {
				args = append(args, "-p", f.ours, "-p", f.incoming)
			}
			other := strings.TrimSpace(string(sparseTestRunGitOutput(t, f.root, args...)))
			if ambiguous {
				ours := strings.TrimSpace(string(sparseTestRunGitOutput(t, f.root, "commit-tree", tree, "-p", f.incoming, "-p", f.ours, "-m", "other merge")))
				sparseTestRunGit(t, f.root, "update-ref", "refs/heads/ours", ours)
			}
			if err := os.WriteFile(f.mergeFile(t), []byte(other+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			before := string(sparseTestRunGitOutput(t, f.root, "rev-parse", "HEAD"))
			if _, err := (Client{}).Commit(context.Background(), f.root, f.opts()); err == nil || !strings.Contains(err.Error(), "merge base") {
				t.Fatalf("unsupported merge base admitted: %v", err)
			}
			if after := string(sparseTestRunGitOutput(t, f.root, "rev-parse", "HEAD")); after != before {
				t.Fatal("HEAD moved")
			}
		})
	}
}

func TestCommitMergeRechecksSnapshotAfterVerifier(t *testing.T) {
	for _, kind := range []string{"HEAD", "MERGE_HEAD", "index bytes", "index mode", "new staged path", "branch"} {
		t.Run(kind, func(t *testing.T) {
			f := newCommitMergeFixture(t, nil, nil)
			var mutatedHead string
			client := Client{Verifier: func(_ string, _ []string) ([]string, error) {
				switch kind {
				case "HEAD":
					sparseTestRunGit(t, f.root, "update-ref", "refs/heads/ours", f.incoming)
				case "MERGE_HEAD":
					if err := os.WriteFile(f.mergeFile(t), []byte(f.ours+"\n"), 0o644); err != nil {
						t.Fatal(err)
					}
				case "index bytes":
					mustWriteGitTestFixture(t, f.root, "incoming.txt", "changed after admission\n")
					sparseTestRunGit(t, f.root, "add", "incoming.txt")
				case "index mode":
					sparseTestRunGit(t, f.root, "update-index", "--chmod=+x", "incoming.txt")
				case "new staged path":
					mustWriteGitTestFixture(t, f.root, "late.txt", "not admitted\n")
					sparseTestRunGit(t, f.root, "add", "late.txt")
				case "branch":
					sparseTestRunGit(t, f.root, "symbolic-ref", "HEAD", "refs/heads/incoming")
				}
				mutatedHead = string(sparseTestRunGitOutput(t, f.root, "rev-parse", "HEAD"))
				return nil, nil
			}}
			if _, err := client.Commit(context.Background(), f.root, f.opts()); err == nil || !strings.Contains(err.Error(), "changed during merge admission") {
				t.Fatalf("changed snapshot accepted: %v", err)
			}
			if after := string(sparseTestRunGitOutput(t, f.root, "rev-parse", "HEAD")); after != mutatedHead {
				t.Fatal("unexpected commit after mutation")
			}
		})
	}
}

func TestCommitMergePreservesUnresolvedAndVerifierVeto(t *testing.T) {
	t.Run("unresolved index", func(t *testing.T) {
		f := newCommitMergeFixture(t, nil, nil)
		// Grant another path so conflict.txt retains its non-zero index stages.
		if _, err := (Client{}).Commit(context.Background(), f.root, f.opts("added.txt")); err == nil || !strings.Contains(err.Error(), "unmerged") {
			t.Fatalf("error = %v", err)
		}
		if after := strings.TrimSpace(string(sparseTestRunGitOutput(t, f.root, "rev-parse", "HEAD"))); after != f.ours {
			t.Fatal("unresolved commit landed")
		}
	})
	t.Run("verifier veto", func(t *testing.T) {
		f := newCommitMergeFixture(t, nil, nil)
		_, err := (Client{Verifier: func(_ string, paths []string) ([]string, error) {
			if !containsCommitPath(paths, "incoming.txt") {
				t.Fatalf("missing admitted path: %q", paths)
			}
			return nil, fmt.Errorf("ticket verifier veto")
		}}).Commit(context.Background(), f.root, f.opts())
		if err == nil || !strings.Contains(err.Error(), "ticket verifier veto") {
			t.Fatalf("error = %v", err)
		}
		if after := strings.TrimSpace(string(sparseTestRunGitOutput(t, f.root, "rev-parse", "HEAD"))); after != f.ours {
			t.Fatal("vetoed commit landed")
		}
		if _, err := os.Stat(f.mergeFile(t)); err != nil {
			t.Fatal("merge state lost")
		}
	})
}
