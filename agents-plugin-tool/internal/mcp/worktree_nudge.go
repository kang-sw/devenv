package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsgit"
)

// The stale-pool nudge's thresholds: config knobs outside the
// worktree.profile. family, validated like declared integer keys.
var (
	worktreeNudgeStaleDaysKey = declaredConfigKey{
		Key:          wsconfig.ItemWorktreeNudgeStaleDays,
		Type:         declaredTypeInteger,
		Minimum:      int64Ptr(1),
		Description:  "Days without activity (the later of the worktree lease's acquired_at and the HEAD commit time) after which worktree.acquire's once-a-day nudge lists a pooled worktree that has a branch checked out. worktree.remove also refuses a leased worktree active more recently than this.",
		DefaultScope: wsconfig.ScopeProject,
		Default:      "7",
		DefaultValue: int64(7),
	}
	worktreeNudgePreservedGBKey = declaredConfigKey{
		Key:          wsconfig.ItemWorktreeNudgePreservedGB,
		Type:         declaredTypeInteger,
		Minimum:      int64Ptr(1),
		Description:  "Gigabytes of preserved ignored artifacts (measured at worktree.release) at or above which worktree.acquire's once-a-day nudge lists a detached pooled worktree.",
		DefaultScope: wsconfig.ScopeProject,
		Default:      "1",
		DefaultValue: int64(1),
	}
)

func int64Ptr(v int64) *int64 { return &v }

// worktreeNudgeThrottleFile, in the git common dir, records per pool when the
// nudge last ran, so it runs at most once per worktreeNudgeInterval.
const (
	worktreeNudgeThrottleFile = "ws-worktree-nudge.json"
	worktreeNudgeInterval     = 24 * time.Hour
	bytesPerGB                = 1_000_000_000
)

// worktreeNudgeThresholds reads the nudge knobs; a value that does not parse
// (hand-edited config) falls back to its default.
func worktreeNudgeThresholds(resolver *wsconfig.Resolver, sessionKey string) (staleAfter time.Duration, preservedLimit int64) {
	read := func(k declaredConfigKey) int64 {
		def := k.DefaultValue.(int64)
		rv, err := resolver.Get(sessionKey, k.Key)
		if err != nil {
			return def
		}
		n, err := strconv.ParseInt(strings.TrimSpace(rv.Value), 10, 64)
		if err != nil || n < *k.Minimum {
			return def
		}
		return n
	}
	return time.Duration(read(worktreeNudgeStaleDaysKey)) * 24 * time.Hour, read(worktreeNudgePreservedGBKey) * bytesPerGB
}

// poolNudge lists poolRoot's worktrees worth removing: a checked-out branch
// with no activity for staleAfter, or a detached worktree whose last release
// measured preservedLimit bytes or more of preserved ignored artifacts. It
// runs at most once per worktreeNudgeInterval per pool and returns nil when
// throttled.
func poolNudge(ctx context.Context, runner wsgit.Runner, root, poolRoot string, staleAfter time.Duration, preservedLimit int64, now time.Time) ([]string, error) {
	commonDir, err := wtRun(ctx, runner, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	throttlePath := filepath.Join(commonDir, worktreeNudgeThrottleFile)
	last := map[string]string{}
	if data, err := os.ReadFile(throttlePath); err == nil {
		_ = json.Unmarshal(data, &last) // a corrupt file only loses the throttle
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if at, err := time.Parse(time.RFC3339, last[poolRoot]); err == nil && now.Sub(at) < worktreeNudgeInterval {
		return nil, nil
	}

	entries, err := listWorktrees(ctx, runner, root)
	if err != nil {
		return nil, err
	}
	var lines []string
	for i, e := range entries {
		if i == 0 || e.Prunable || !pathUnder(poolRoot, e.Path) {
			continue
		}
		if !e.Detached {
			lease, _ := readWorktreeLease(ctx, runner, e.Path)
			activity, err := worktreeActivity(ctx, runner, e.Path, lease)
			if err == nil && now.Sub(activity) >= staleAfter {
				lines = append(lines, fmt.Sprintf("%s: branch %s, no activity for %d days (last %s)", e.Path, strings.TrimPrefix(e.Branch, "refs/heads/"), int(now.Sub(activity).Hours()/24), formatFactTime(activity)))
			}
			continue
		}
		marker, err := readWorktreeMarker(ctx, runner, e.Path)
		if err == nil && marker != nil && marker.PreservedBytes >= preservedLimit {
			lines = append(lines, fmt.Sprintf("%s: detached, %.1f GB of preserved ignored artifacts (measured %s)", e.Path, float64(marker.PreservedBytes)/bytesPerGB, marker.PreservedMeasuredAt))
		}
	}

	last[poolRoot] = now.UTC().Format(time.RFC3339)
	if data, err := json.Marshal(last); err == nil {
		tmp := throttlePath + ".tmp"
		if os.WriteFile(tmp, data, 0o644) == nil {
			_ = os.Rename(tmp, throttlePath)
		}
	}
	return lines, nil
}

// preservedIgnoredBytes sums the regular-file bytes under every ignored path
// git status reports. Called right after release hygiene, when the only
// ignored paths left are the ones the profile preserves.
func preservedIgnoredBytes(ctx context.Context, runner wsgit.Runner, wtPath string) (int64, error) {
	raw, err := runner.RunGit(ctx, wtPath, "--no-optional-locks", "status", "--porcelain", "--ignored", "-z")
	if err != nil {
		return 0, err
	}
	var total int64
	for _, rec := range strings.Split(string(raw), "\x00") {
		rel, ok := strings.CutPrefix(rec, "!! ")
		if !ok {
			continue
		}
		root := filepath.Join(wtPath, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
		err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // a vanished entry only shrinks the estimate
			}
			if d.Type().IsRegular() {
				if info, err := d.Info(); err == nil {
					total += info.Size()
				}
			}
			return nil
		})
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
