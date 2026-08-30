package wsreview

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWriteAgentsMD(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func TestReadAgentsReviewPolicyDefaultsWhenFileAbsent(t *testing.T) {
	root := t.TempDir()
	got := ReadAgentsReviewPolicy(root)
	want := AgentsReviewPolicy{
		ReleaseBoundary:   ReleaseBoundaryAbsent,
		RendezvousBackend: RendezvousBackendCanary,
		ReleaseTagGlob:    DefaultReleaseTagGlob,
	}
	if got != want {
		t.Fatalf("ReadAgentsReviewPolicy = %+v, want %+v", got, want)
	}
}

func TestReadAgentsReviewPolicyDefaultsWhenSectionAbsent(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "# AGENTS.md\n\n## Workflow\n\n### Branch Policy\nsome unrelated content\n")
	got := ReadAgentsReviewPolicy(root)
	want := AgentsReviewPolicy{
		ReleaseBoundary:   ReleaseBoundaryAbsent,
		RendezvousBackend: RendezvousBackendCanary,
		ReleaseTagGlob:    DefaultReleaseTagGlob,
	}
	if got != want {
		t.Fatalf("ReadAgentsReviewPolicy = %+v, want %+v", got, want)
	}
}

func TestReadAgentsReviewPolicyDefaultsWhenFieldsAbsent(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "# AGENTS.md\n\n## Workflow\n\n### Review Policy\n(nothing declared yet)\n")
	got := ReadAgentsReviewPolicy(root)
	want := AgentsReviewPolicy{
		ReleaseBoundary:   ReleaseBoundaryAbsent,
		RendezvousBackend: RendezvousBackendCanary,
		ReleaseTagGlob:    DefaultReleaseTagGlob,
	}
	if got != want {
		t.Fatalf("ReadAgentsReviewPolicy = %+v, want %+v", got, want)
	}
}

func TestReadAgentsReviewPolicyParsesAllFieldsPresent(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "# AGENTS.md\n\n## Workflow\n\n### Review Policy\nreview-track: develop\nrelease-boundary: present\nrendezvous-backend: canary\nrelease-tag-glob: v*\n\n### Next Section\nother content\n")
	got := ReadAgentsReviewPolicy(root)
	want := AgentsReviewPolicy{
		ReviewTrack:       "develop",
		ReleaseBoundary:   ReleaseBoundaryPresent,
		RendezvousBackend: RendezvousBackendCanary,
		ReleaseTagGlob:    "v*",
	}
	if got != want {
		t.Fatalf("ReadAgentsReviewPolicy = %+v, want %+v", got, want)
	}
}

func TestReadAgentsReviewPolicyParsesPlatformBackend(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "## Workflow\n\n### Review Policy\nreview-track: main\nrelease-boundary: present\nrendezvous-backend: platform\nrelease-tag-glob: release-*\n")
	got := ReadAgentsReviewPolicy(root)
	if got.RendezvousBackend != RendezvousBackendPlatform {
		t.Fatalf("RendezvousBackend = %q, want %q", got.RendezvousBackend, RendezvousBackendPlatform)
	}
	if got.ReleaseTagGlob != "release-*" {
		t.Fatalf("ReleaseTagGlob = %q, want %q", got.ReleaseTagGlob, "release-*")
	}
}

func TestReadAgentsReviewPolicyFailsOpenOnMalformedReleaseBoundary(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "## Workflow\n\n### Review Policy\nreview-track: develop\nrelease-boundary: maybe\nrendezvous-backend: canary\n")
	got := ReadAgentsReviewPolicy(root)
	if got.ReleaseBoundary != ReleaseBoundaryAbsent {
		t.Fatalf("ReleaseBoundary = %q, want fail-open default %q", got.ReleaseBoundary, ReleaseBoundaryAbsent)
	}
}

func TestReadAgentsReviewPolicyFailsOpenOnMalformedRendezvousBackend(t *testing.T) {
	root := t.TempDir()
	mustWriteAgentsMD(t, root, "## Workflow\n\n### Review Policy\nreview-track: develop\nrelease-boundary: present\nrendezvous-backend: carrier-pigeon\n")
	got := ReadAgentsReviewPolicy(root)
	if got.RendezvousBackend != RendezvousBackendCanary {
		t.Fatalf("RendezvousBackend = %q, want fail-open default %q", got.RendezvousBackend, RendezvousBackendCanary)
	}
}
