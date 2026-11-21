package wsagent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// PiBackend runs Pi in RPC mode, one process per conversation, on a
// session file kept in the conversation's session directory, so a restart
// resumes the same Pi session. It keeps Pi's own system prompt and appends
// the composed instruction file; ambient extensions, skills, prompt
// templates, context files, and trust-gated project configuration stay
// off, and --tools is the whole tool surface.
type PiBackend struct {
	// Command is the pi executable; empty looks up "pi" on PATH.
	Command string
}

// piEfforts are pi's thinking levels (--thinking), lowest first.
var piEfforts = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// runListing runs a harness's model listing and returns its stdout; a test
// replaces it with a fixture.
var runListing = func(ctx context.Context, command string, args []string, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	// A grandchild that inherited stdout (a Windows launcher shim's) must
	// not hold the listing, and Support's choicesMu, past ctx's timeout.
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

// Executable is the pi command availability is checked against.
func (b PiBackend) Executable() string {
	if b.Command != "" {
		return b.Command
	}
	return "pi"
}

// Choices lists the models pi --list-models reports, offline and with
// project-local files ignored, and pi's thinking levels. The levels come
// back even when the listing fails.
func (b PiBackend) Choices(ctx context.Context) ([]ModelChoice, []string, error) {
	models, efforts, _, err := b.choicesObserved(ctx, piProbeNow)
	return models, efforts, err
}

// choicesObserved carries a disk hit's original observation into Support's
// memory cache, so a restart cannot renew a nearly expired listing.
func (b PiBackend) choicesObserved(ctx context.Context, now func() time.Time) ([]ModelChoice, []string, time.Time, error) {
	efforts := append([]string(nil), piEfforts...)
	command, err := lookPath(b.Executable())
	if err != nil {
		return nil, efforts, time.Time{}, fmt.Errorf("pi backend: %w", err)
	}
	env := piEnv(environ())
	cache := newPiProbeCache("models", command, nil, env)
	if record, ok := cache.load(now()); ok {
		if models, ok := record.models(); ok {
			return models, efforts, record.ObservedAt, nil
		}
	}
	out, err := runListing(ctx, command, []string{"--offline", "--no-approve", "--list-models"}, env)
	if err != nil {
		return nil, efforts, time.Time{}, fmt.Errorf("pi --list-models: %w", err)
	}
	models, err := parsePiModels(out)
	at := now()
	if err == nil {
		cache.save(at, models)
	}
	return models, efforts, at, err
}

// parsePiModels reads pi --list-models' table: a header naming the
// columns (provider, model, ..., thinking, ...) and one whitespace-separated
// row per model. A model's id is provider/model, as --model takes it; a row
// listed twice is kept once, and an id the runner would refuse is skipped.
func parsePiModels(out []byte) ([]ModelChoice, error) {
	var cols map[string]int
	models := []ModelChoice{}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if cols == nil {
			if fields[0] != "provider" {
				continue
			}
			cols = map[string]int{}
			for i, name := range fields {
				cols[name] = i
			}
			if _, ok := cols["model"]; !ok {
				return nil, fmt.Errorf("pi --list-models header has no model column: %q", strings.TrimSpace(line))
			}
			continue
		}
		if len(fields) <= cols["model"] {
			continue
		}
		id := fields[cols["provider"]] + "/" + fields[cols["model"]]
		if seen[id] || !modelPattern.MatchString(id) {
			continue
		}
		seen[id] = true
		m := ModelChoice{ID: id}
		if i, ok := cols["thinking"]; ok && i < len(fields) {
			thinking := fields[i] == "yes"
			m.Thinking = &thinking
		}
		models = append(models, m)
	}
	if cols == nil && strings.TrimSpace(string(out)) != "" {
		first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
		return nil, fmt.Errorf("pi --list-models printed no model table: %q", first)
	}
	return models, nil
}

// piBridgeFile is the extension the runner writes into a conversation's Pi
// directory when the profile allows MCP servers (ws-mcp included): it
// registers exactly those servers through Pi's built-in MCP support.
const piBridgeFile = "ws-agent-mcp.ts"

// PiWaitMax caps pi.wait in both the MCP tool and the token route.
const PiWaitMax = 1800 * time.Second

// PiForwardSlack leaves time for the daemon to answer a forwarded wait.
const PiForwardSlack = 30 * time.Second

// piMCPTimeout exceeds even a maximum forwarded wait, with transport margin.
// It also covers the launcher's 75s unsent-request wait, including initialize
// and tools/list, which Pi's default 60s request timeout would cut off.
const piMCPTimeout = PiWaitMax + PiForwardSlack + 30*time.Second

// piDir is where a conversation's Pi session lives.
func piDir(dir string) string { return filepath.Join(dir, "pi") }

func piSessionFile(dir string) string { return filepath.Join(piDir(dir), "session.jsonl") }

// PiSessionAvailable reports whether a conversation's session directory holds
// the regular Pi session file required to revive a closed child. Stat errors,
// including inaccessible files, fail the same check as a missing file.
func PiSessionAvailable(sessionDir string) bool {
	info, err := os.Stat(piSessionFile(sessionDir))
	return err == nil && info.Mode().IsRegular()
}

var piNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// piMCPToolName is Pi's name for an MCP server's tool: mcp__<server>__<tool>
// with every character outside [A-Za-z0-9_] as an underscore.
func piMCPToolName(server, tool string) string {
	return "mcp__" + piNameUnsafe.ReplaceAllString(server, "_") + "__" + piNameUnsafe.ReplaceAllString(tool, "_")
}

// piSurface is the tool surface a Pi child may declare: the --tools entries,
// and the names and MCP-server prefixes the tool check accepts.
type piSurface struct {
	entries  []string
	exact    map[string]bool
	prefixes []string
}

func piToolSurface(spec StartSpec) piSurface {
	s := piSurface{exact: map[string]bool{}}
	add := func(name string) {
		if !s.exact[name] {
			s.exact[name] = true
			s.entries = append(s.entries, name)
		}
	}
	for _, entry := range spec.Profile.Tools {
		add(ToolBase(entry))
	}
	if spec.WSMCP != nil {
		for _, name := range spec.Profile.WSTools {
			add(piMCPToolName(ReservedMCPServer, name))
		}
	}
	for _, name := range spec.Profile.MCPServerNames() {
		prefix := "mcp__" + piNameUnsafe.ReplaceAllString(name, "_") + "__"
		s.prefixes = append(s.prefixes, prefix)
		s.entries = append(s.entries, prefix+"*")
	}
	return s
}

func (s piSurface) allows(tool string) bool {
	if s.exact[tool] {
		return true
	}
	for _, prefix := range s.prefixes {
		if strings.HasPrefix(tool, prefix) {
			return true
		}
	}
	return false
}

// checkPiTools refuses a tool loadout that declares a tool outside the
// profile. Pi reports its loadout in system messages: the first request's
// carries every declared tool, later ones the tools added since.
func checkPiTools(added []string, s piSurface) error {
	var extra []string
	for _, name := range added {
		if !s.allows(name) {
			extra = append(extra, name)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	sort.Strings(extra)
	return fmt.Errorf("launch refused: pi reports tools outside the profile: %s", strings.Join(extra, ", "))
}

// piVersionPattern finds a dotted version in `pi --version` output.
var piVersionPattern = regexp.MustCompile(`(\d+)\.(\d+)(?:\.\d+)?`)

// runPiVersion is separate from the model-listing seam so fixtures can
// forbid either subprocess independently without changing its argv.
var runPiVersion = func(ctx context.Context, command string, args, env []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = env
	return cmd.Output()
}

// PiVersion runs `pi --version` (pi looked up on PATH, as PiBackend does
// with no Command) and returns its first dotted version, such as "1.0.4".
// It fails when pi is missing, does not exit within the context, or prints
// no version.
func PiVersion(ctx context.Context) (string, error) {
	command, pre, err := harnessCommand("", "pi")
	if err != nil {
		return "", err
	}
	env := piEnv(environ())
	cache := newPiProbeCache("version", command, pre, env)
	if record, ok := cache.load(piProbeNow()); ok {
		if version, ok := record.version(); ok {
			return version, nil
		}
	}
	out, err := runPiVersion(ctx, command, append(pre, "--version"), env)
	if err != nil {
		return "", fmt.Errorf("pi --version: %w", err)
	}
	version, err := parsePiVersion(out)
	if err == nil {
		cache.save(piProbeNow(), version)
	}
	return version, err
}

func parsePiVersion(out []byte) (string, error) {
	version := piVersionPattern.FindString(string(out))
	if version == "" {
		return "", fmt.Errorf("pi --version printed no version: %q", strings.TrimSpace(string(out)))
	}
	return version, nil
}

// VersionAtLeast reports whether version's major and minor numbers (its
// first dotted pair) are at least major.minor; a version it cannot read is
// not.
func VersionAtLeast(version string, major, minor int) bool {
	m := piVersionPattern.FindStringSubmatch(version)
	if m == nil {
		return false
	}
	gotMajor, _ := strconv.Atoi(m[1])
	gotMinor, _ := strconv.Atoi(m[2])
	return gotMajor > major || (gotMajor == major && gotMinor >= minor)
}
