package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsagent"
	"github.com/kang-sw/devenv/internal/wsrsrc"
)

const (
	apiDocManagerPrompt = "api-doc-manager"
	apiPreRouterPrompt  = "pre-router"
	apiCargoBriefPrompt = "api-doc-cargo-brief"
	apiManagerPrefix    = "api-doc-"
	apiAskTimeout       = 10 * time.Minute
	apiManagerTTL       = 5 * time.Minute
)

type apiRuntime interface {
	Route(ctx context.Context, root, prompt string) (string, error)
	AskManager(ctx context.Context, root, domain, prompt string) (string, error)
}

type wsagentAPIRuntime struct {
	harness string
}

type apiDomainResult struct {
	domain string
	text   string
	err    error
}

// renderAPIPrompt loads an api-doc prompt from the rsrc tree and returns its
// body. The api-doc prompts (pre-router, api-doc-manager, api-doc-cargo-brief)
// are var-free `kind: print` rsrc playbooks, so a nil-vars Load returns the
// verbatim body. Phase 6 (260611) moved these off the embedded wsprompt bundle.
func renderAPIPrompt(harness, stem string) (string, error) {
	rsrcRoot, err := wsrsrc.ResolveRoot()
	if err != nil {
		return "", fmt.Errorf("resolve rsrc root: %w", err)
	}
	pb, err := wsrsrc.Load(rsrcRoot, stem, harness, nil)
	if err != nil {
		return "", fmt.Errorf("load api prompt %q: %w", stem, err)
	}
	return pb.Body, nil
}

func (rt wsagentAPIRuntime) Route(ctx context.Context, root, prompt string) (string, error) {
	mgr := wsagent.NewManager(wsagent.Options{})
	name := fmt.Sprintf("api-doc-pre-router-%d", time.Now().UTC().UnixNano())
	sys, err := renderAPIPrompt(rt.harness, apiPreRouterPrompt)
	if err != nil {
		return "", err
	}
	_, _, err = mgr.Register(wsagent.RegisterOptions{
		Root:                root,
		Name:                name,
		Harness:             rt.harness,
		Model:               "light",
		SystemPromptText:    sys,
		SuppressOrientation: true,
	})
	if err != nil {
		return "", err
	}
	defer func() { _ = mgr.Erase(root, name) }()
	if _, err := mgr.Call(wsagent.CallOptions{Root: root, Name: name, Prompt: prompt}); err != nil {
		return "", err
	}
	return resultWithManagerCancel(ctx, mgr, root, name)
}

func (rt wsagentAPIRuntime) AskManager(ctx context.Context, root, domain, prompt string) (string, error) {
	mgr := wsagent.NewManager(wsagent.Options{})
	name := apiManagerName(domain)
	agent, active, err := mgr.Inspect(root, name)
	if err == nil && apiManagerExpired(agent, time.Now().UTC()) && !active {
		if eraseErr := mgr.Erase(root, name); eraseErr != nil {
			return "", eraseErr
		}
		err = os.ErrNotExist
	}
	if err != nil {
		sys, renderErr := renderAPIPrompt(rt.harness, apiDocManagerPrompt)
		if renderErr != nil {
			return "", renderErr
		}
		// Conditional cargo-brief: append the cargo-brief guidance only when the
		// binary is present (replaces the former ConditionalPromptRef, which
		// resolved through the embedded bundle).
		if _, lookErr := exec.LookPath("cargo-brief"); lookErr == nil {
			brief, briefErr := renderAPIPrompt(rt.harness, apiCargoBriefPrompt)
			if briefErr != nil {
				return "", briefErr
			}
			sys = sys + "\n\n" + brief
		}
		if _, _, regErr := mgr.Register(wsagent.RegisterOptions{
			Root:                root,
			Name:                name,
			Harness:             rt.harness,
			Model:               "core",
			SystemPromptText:    sys,
			SuppressOrientation: true,
		}); regErr != nil {
			return "", regErr
		}
	}
	if _, err := mgr.Call(wsagent.CallOptions{Root: root, Name: name, Prompt: prompt}); err != nil {
		return "", err
	}
	return resultWithManagerCancel(ctx, mgr, root, name)
}

func resultWithManagerCancel(ctx context.Context, mgr wsagent.Manager, root, name string) (string, error) {
	if ctx == nil {
		return mgr.Result(wsagent.ResultOptions{Root: root, Name: name, Timeout: apiAskTimeout})
	}
	done := make(chan struct{})
	cancelled := make(chan struct{})
	var cancelOnce sync.Once
	cancelManager := func() {
		cancelOnce.Do(func() {
			_, _ = mgr.Cancel(root, name)
		})
	}
	go func() {
		defer close(cancelled)
		select {
		case <-ctx.Done():
			cancelManager()
		case <-done:
		}
	}()
	text, err := mgr.Result(wsagent.ResultOptions{Root: root, Name: name, Timeout: apiAskTimeout, Context: ctx})
	if ctx.Err() != nil {
		cancelManager()
	}
	close(done)
	if ctx.Err() != nil {
		<-cancelled
	}
	return text, err
}

func apiManagerExpired(agent wsagent.Agent, now time.Time) bool {
	for _, value := range []string{agent.LastCallAt, agent.LastSeenAt, agent.CreatedAt} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			continue
		}
		return now.Sub(t) > apiManagerTTL
	}
	return false
}

var apiDomainLocks sync.Map // map[string]*sync.Mutex; process-local guard for same-domain MCP calls.

var apiDomainSlugPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

func apiListDomains(root string) ([]string, error) {
	entries, err := os.ReadDir(apiDepsDir(root))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	domains := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		domains = append(domains, name)
	}
	sort.Strings(domains)
	return domains, nil
}

func (s *Server) askAPI(ctx context.Context, root, prompt, hint string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", errors.New("prompt is required")
	}
	domains, err := s.resolveAPIDomains(ctx, root, prompt, hint)
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return "", errors.New("api.ask resolved no domains")
	}

	results := make([]apiDomainResult, len(domains))
	var wg sync.WaitGroup
	for i, domain := range domains {
		i, domain := i, domain
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i].domain = domain
			if err := os.MkdirAll(filepath.Join(apiDepsDir(root), domain), 0o755); err != nil {
				results[i].err = err
				return
			}
			lock := apiLockFor(root, domain)
			lock.Lock()
			defer lock.Unlock()
			text, err := s.apiRuntime().AskManager(ctx, root, domain, prompt)
			results[i].text = text
			results[i].err = err
		}()
	}
	wg.Wait()

	text, successes := formatAPIResults(results)
	if successes == 0 {
		return text, errors.New("api.ask failed for all resolved domains")
	}
	return text, nil
}

func formatAPIResults(results []apiDomainResult) (string, int) {
	successes := 0
	var b strings.Builder
	b.WriteString("api.ask results\n")
	for _, res := range results {
		b.WriteString("\n## Domain: ")
		b.WriteString(res.domain)
		b.WriteString("\n")
		if res.err != nil {
			b.WriteString("ERROR: ")
			b.WriteString(res.err.Error())
			b.WriteByte('\n')
			continue
		}
		successes++
		b.WriteString(strings.TrimRight(res.text, "\n"))
		b.WriteByte('\n')
	}
	return b.String(), successes
}

func (s *Server) resolveAPIDomains(ctx context.Context, root, prompt, hint string) ([]string, error) {
	existing, err := apiListDomains(root)
	if err != nil {
		return nil, err
	}
	hint = strings.TrimSpace(hint)
	if hint != "" {
		for _, domain := range existing {
			if hint == domain {
				return []string{domain}, nil
			}
		}
	}
	input := formatAPIPreRouterPrompt(hint, existing, prompt)
	output, err := s.apiRuntime().Route(ctx, root, input)
	if err != nil {
		return nil, fmt.Errorf("api pre-router failed: %w", err)
	}
	return parseAPIRouterDomains(output, existing)
}

func formatAPIPreRouterPrompt(hint string, existing []string, prompt string) string {
	if strings.TrimSpace(hint) == "" {
		hint = "(none)"
	}
	var b strings.Builder
	b.WriteString("Hint: ")
	b.WriteString(hint)
	b.WriteString("\nExisting domains:\n")
	for _, domain := range existing {
		b.WriteString(domain)
		b.WriteByte('\n')
	}
	b.WriteString("Prompt: ")
	b.WriteString(prompt)
	b.WriteByte('\n')
	return b.String()
}

func parseAPIRouterDomains(output string, existing []string) ([]string, error) {
	seen := map[string]bool{}
	var domains []string
	var invalid []string
	for _, line := range strings.Split(output, "\n") {
		domain := strings.TrimSpace(line)
		if domain == "" {
			continue
		}
		if !validAPIDomain(domain) {
			invalid = append(invalid, domain)
			continue
		}
		if !seen[domain] {
			seen[domain] = true
			domains = append(domains, domain)
		}
	}
	if len(domains) == 0 {
		domains = matchExistingAPIDomains(output, existing)
	}
	if len(domains) == 0 {
		if len(invalid) > 0 {
			return nil, fmt.Errorf("api pre-router returned invalid domain %q", invalid[0])
		}
		return nil, errors.New("api pre-router returned no domains")
	}
	return domains, nil
}

func matchExistingAPIDomains(prompt string, existing []string) []string {
	var matched []string
	for _, domain := range existing {
		if apiDomainMentioned(prompt, domain) {
			matched = append(matched, domain)
		}
	}
	return matched
}

func apiDomainMentioned(prompt, domain string) bool {
	if domain == "" {
		return false
	}
	pattern := `(?i)(^|[^A-Za-z0-9._-])` + regexp.QuoteMeta(domain) + `($|[^A-Za-z0-9._-])`
	return regexp.MustCompile(pattern).FindStringIndex(prompt) != nil
}

func validAPIDomain(domain string) bool {
	if domain == "" || domain == "." || domain == ".." || strings.HasPrefix(domain, ".") {
		return false
	}
	return apiDomainSlugPattern.MatchString(domain)
}

func apiDepsDir(root string) string {
	return filepath.Join(root, "ai-docs", ".deps")
}

func apiManagerName(domain string) string {
	return apiManagerPrefix + domain
}

func apiLockFor(root, domain string) *sync.Mutex {
	key := filepath.Clean(root) + "\x00" + domain
	lock, _ := apiDomainLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (s *Server) apiRuntime() apiRuntime {
	if s.api != nil {
		return s.api
	}
	return wsagentAPIRuntime{harness: s.currentHarness()}
}
