package wsagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kang-sw/devenv/internal/wsconfig"
	"github.com/kang-sw/devenv/internal/wsdashboard"
)

// SupportRoutePrefix is where the daemon mounts the tracker support
// conversations: one ongoing runner conversation per tracker store, opened
// from the store's chat card.
//
//	GET  /support/choices               harnesses, their models and efforts, the last choice
//	GET  /support/stores/<store>        the store's conversation (or null) and the selection
//	POST /support/stores/<store>/send   {text}; opens the conversation first when none is open
//	POST /support/stores/<store>/new    closes the open conversation and opens a fresh one
//	POST /support/stores/<store>/select {harness?, model?, effort?}
//
// Every POST follows the runner routes' rule (writeAllowed): a JSON body,
// and no Origin or the daemon's own. An unknown store is 404.
const SupportRoutePrefix = wsdashboard.SupportRoutePrefix

// supportFileName is the support state file in the dashboard state
// directory: the last selection and the store-to-conversation mapping.
const supportFileName = "support.json"

const supportSchemaVersion = 1

// supportProfiles are the support profiles in default order: the first
// available harness is the default selection.
var supportProfiles = []struct{ name, backend string }{
	{SupportProfileClaude, BackendClaude},
	{SupportProfilePi, BackendPi},
}

// choicesTTL is how long a harness's listed choices are reused.
const choicesTTL = 5 * time.Minute

// choicesFailTTL is how long a failed listing is reused, so a harness whose
// listing hangs does not hold every choices request for choicesTimeout.
const choicesFailTTL = 30 * time.Second

// choicesTimeout bounds one harness's listing.
const choicesTimeout = 15 * time.Second

// ModelChoice is one model a harness offers, as --model takes it.
type ModelChoice struct {
	ID string `json:"id"`
	// Thinking reports whether the model takes a reasoning effort; absent
	// when the harness does not say.
	Thinking *bool `json:"thinking,omitempty"`
}

// ChoiceLister is a Backend that can tell what it offers: the executable
// its availability is checked against, and its models and effort levels. A
// backend without it is available whenever registered, with no choices.
type ChoiceLister interface {
	Executable() string
	// Choices lists the models and the effort levels, lowest first. A
	// listing failure still returns the efforts it knows.
	Choices(ctx context.Context) ([]ModelChoice, []string, error)
}

// Selection is a harness, model, and effort choice; an empty model or
// effort is the profile's, else the harness's, default.
type Selection struct {
	Harness string `json:"harness"`
	Model   string `json:"model"`
	Effort  string `json:"effort"`
}

// supportState is support.json.
type supportState struct {
	SchemaVersion int `json:"schema_version"`
	// Last is the machine-wide last selection, one across stores; nil until
	// one is made.
	Last *Selection `json:"last,omitempty"`
	// Stores maps a store id to its conversation's reply-id.
	Stores map[string]string `json:"stores"`
}

// SupportOptions configures a Support.
type SupportOptions struct {
	Runner *Runner
	// Dir is the dashboard state directory support.json lives in.
	Dir string
	// StoreRoot returns a tracker store's working root (its registry
	// entry's directory, which the tracker gate pins the conversation's
	// writes to); an error is an unknown store.
	StoreRoot func(id string) (string, error)
	// Config is the base of the support profiles' resolution for the
	// harness list (global and builtin layers only); zero uses the
	// runner's.
	Config wsconfig.Options
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

// Support keeps one ongoing support conversation per tracker store over
// the runner, and the last harness, model, and effort selection.
type Support struct {
	opts SupportOptions

	// mu serializes every state change and conversation operation.
	mu    sync.Mutex
	state supportState

	// choicesMu guards cache; a listing runs under it, so concurrent
	// requests share one.
	choicesMu sync.Mutex
	cache     map[string]cachedChoices
}

type cachedChoices struct {
	at      time.Time
	models  []ModelChoice
	efforts []string
	// err is a failed listing's message; such an entry answers for
	// choicesFailTTL only.
	err string
}

// fresh reports whether c still answers at now.
func (c cachedChoices) fresh(now time.Time) bool {
	if c.err != "" {
		return now.Sub(c.at) < choicesFailTTL
	}
	return now.Sub(c.at) < choicesTTL
}

// NewSupport returns a Support over opts, loading support.json; a missing
// or unreadable file starts empty.
func NewSupport(opts SupportOptions) *Support {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Config == (wsconfig.Options{}) {
		opts.Config = opts.Runner.opts.Config
	}
	opts.Config.RepoRoot = ""
	s := &Support{opts: opts, cache: map[string]cachedChoices{}}
	raw, err := os.ReadFile(s.path())
	if err == nil {
		err = json.Unmarshal(raw, &s.state)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(opts.Runner.opts.Log, "support: ignore %s: %v\n", s.path(), err)
		s.state = supportState{}
	}
	if s.state.Stores == nil {
		s.state.Stores = map[string]string{}
	}
	return s
}

func (s *Support) path() string { return filepath.Join(s.opts.Dir, supportFileName) }

// save writes support.json; s.mu is held.
func (s *Support) save() error {
	s.state.SchemaVersion = supportSchemaVersion
	raw, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(s.path(), append(raw, '\n'), 0o600)
}

// SupportHarness is one support harness: a support profile's resolved
// backend.
type SupportHarness struct {
	Harness   string `json:"harness"`
	Profile   string `json:"profile"`
	Available bool   `json:"available"`
	// Error is why the harness is unavailable, or why its model listing
	// failed.
	Error   string        `json:"error,omitempty"`
	Models  []ModelChoice `json:"models"`
	Efforts []string      `json:"efforts"`
}

// harnesses resolves the support profiles into the harness list, in
// default order, without listing choices. Each profile is resolved through
// the global and builtin layers; one whose backend another already took is
// left out.
func (s *Support) harnesses() []SupportHarness {
	var out []SupportHarness
	seen := map[string]bool{}
	for _, sp := range supportProfiles {
		h := SupportHarness{Harness: sp.backend, Profile: sp.name, Models: []ModelChoice{}, Efforts: []string{}}
		rp, err := ResolveProfile(s.opts.Config, "", sp.name)
		if err == nil {
			h.Harness = rp.Definition.Backend
		}
		if seen[h.Harness] {
			continue
		}
		seen[h.Harness] = true
		switch backend := s.opts.Runner.opts.Backends[h.Harness]; {
		case err != nil:
			h.Error = err.Error()
		case backend == nil:
			h.Error = fmt.Sprintf("backend %q is not available in this daemon", h.Harness)
		default:
			h.Available = true
			if cl, ok := backend.(ChoiceLister); ok {
				if _, err := lookPath(cl.Executable()); err != nil {
					h.Available, h.Error = false, fmt.Sprintf("%s is not installed: %v", cl.Executable(), err)
				}
			}
		}
		out = append(out, h)
	}
	return out
}

// listChoices fills each available harness's models and efforts, from the
// cache when fresh. A listing failure is that harness's error alone, and is
// cached briefly too. A listing runs apart from ctx's cancellation, bounded
// by choicesTimeout alone: a request its caller abandoned still fills the
// cache for the next one instead of leaving it to list again.
func (s *Support) listChoices(ctx context.Context, hs []SupportHarness) {
	s.choicesMu.Lock()
	defer s.choicesMu.Unlock()
	for i := range hs {
		h := &hs[i]
		if !h.Available {
			continue
		}
		cl, ok := s.opts.Runner.opts.Backends[h.Harness].(ChoiceLister)
		if !ok {
			continue
		}
		if c, ok := s.cache[h.Harness]; ok && c.fresh(s.opts.Now()) {
			h.Models, h.Efforts, h.Error = c.models, c.efforts, c.err
			continue
		}
		lctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), choicesTimeout)
		models, efforts, err := cl.Choices(lctx)
		cancel()
		// Stamped when the listing ended, not when it began: a listing that
		// hung until choicesTimeout would otherwise cache an entry already
		// that old.
		now := s.opts.Now()
		if efforts != nil {
			h.Efforts = efforts
		}
		if err != nil {
			h.Error = err.Error()
			s.cache[h.Harness] = cachedChoices{at: now, models: h.Models, efforts: h.Efforts, err: h.Error}
			continue
		}
		if models != nil {
			h.Models = models
		}
		s.cache[h.Harness] = cachedChoices{at: now, models: h.Models, efforts: h.Efforts}
	}
}

// findHarness returns the harness named name in hs.
func findHarness(hs []SupportHarness, name string) (SupportHarness, bool) {
	for _, h := range hs {
		if h.Harness == name {
			return h, true
		}
	}
	return SupportHarness{}, false
}

// selection is the effective selection: the last one when its harness is
// available, else the first available harness with default model and
// effort, else the empty selection. s.mu is held.
func (s *Support) selection(hs []SupportHarness) Selection {
	if last := s.state.Last; last != nil {
		if h, ok := findHarness(hs, last.Harness); ok && h.Available {
			return *last
		}
	}
	for _, h := range hs {
		if h.Available {
			return Selection{Harness: h.Harness}
		}
	}
	return Selection{}
}

var errNoHarness = &requestError{status: http.StatusServiceUnavailable, msg: "no support harness is available: neither claude nor pi is installed and registered"}

// current returns the store's open conversation, if any: the mapped one,
// still open, at root. s.mu is held.
func (s *Support) current(store, root string) (State, bool) {
	id := s.state.Stores[store]
	if id == "" {
		return State{}, false
	}
	st, err := s.opts.Runner.Get(id)
	if err != nil || st.Status == StatusClosed || st.Root != filepath.Clean(root) {
		return State{}, false
	}
	return st, true
}

// open creates a conversation at root with sel, maps it to store, and
// saves. s.mu is held.
func (s *Support) open(store, root string, sel Selection, hs []SupportHarness) (State, error) {
	h, ok := findHarness(hs, sel.Harness)
	if !ok || !h.Available {
		return State{}, errNoHarness
	}
	st, err := s.opts.Runner.Create(h.Profile, root)
	if err != nil {
		return State{}, err
	}
	var model, effort *string
	if sel.Model != "" {
		model = &sel.Model
	}
	if sel.Effort != "" {
		effort = &sel.Effort
	}
	if model != nil || effort != nil {
		// SetSettings answers State{} on error, so close by the created id.
		created := st.ID
		if st, err = s.opts.Runner.SetSettings(created, model, effort); err != nil {
			_, _ = s.opts.Runner.CloseConversation(created)
			return State{}, err
		}
	}
	s.state.Stores[store] = st.ID
	if err := s.save(); err != nil {
		return State{}, err
	}
	return st, nil
}

// StoreAnswer is a store's support state.
type StoreAnswer struct {
	Store string `json:"store"`
	// Conversation is the store's open conversation; null when none is.
	Conversation *State    `json:"conversation"`
	Selection    Selection `json:"selection"`
}

// SelectAnswer is a select's answer.
type SelectAnswer struct {
	Conversation *State    `json:"conversation"`
	Selection    Selection `json:"selection"`
}

// ChoicesAnswer is the choices route's answer.
type ChoicesAnswer struct {
	Harnesses []SupportHarness `json:"harnesses"`
	// Last is the selection a new conversation starts with.
	Last Selection `json:"last"`
}

// Choices lists the harnesses with their models and efforts, and the
// selection a new conversation starts with.
func (s *Support) Choices(ctx context.Context) ChoicesAnswer {
	hs := s.harnesses()
	s.listChoices(ctx, hs)
	s.mu.Lock()
	defer s.mu.Unlock()
	return ChoicesAnswer{Harnesses: hs, Last: s.selection(hs)}
}

// Store reads the store's conversation and the current selection.
func (s *Support) Store(store string) (StoreAnswer, error) {
	root, err := s.root(store)
	if err != nil {
		return StoreAnswer{}, err
	}
	hs := s.harnesses()
	s.mu.Lock()
	defer s.mu.Unlock()
	ans := StoreAnswer{Store: store, Selection: s.selection(hs)}
	if st, ok := s.current(store, root); ok {
		ans.Conversation = &st
	}
	return ans, nil
}

// Send sends text to the store's conversation, opening one with the
// current selection when none is open.
func (s *Support) Send(store, text string) (State, error) {
	if strings.TrimSpace(text) == "" {
		return State{}, badRequest("text is empty")
	}
	root, err := s.root(store)
	if err != nil {
		return State{}, err
	}
	hs := s.harnesses()
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.current(store, root)
	if !ok {
		if st, err = s.open(store, root, s.selection(hs), hs); err != nil {
			return State{}, err
		}
	}
	return s.opts.Runner.Send(st.ID, text)
}

// New closes the store's open conversation, if any, and opens a fresh one
// with the current selection; it sends nothing.
func (s *Support) New(store string) (State, error) {
	root, err := s.root(store)
	if err != nil {
		return State{}, err
	}
	hs := s.harnesses()
	s.mu.Lock()
	defer s.mu.Unlock()
	if st, ok := s.current(store, root); ok {
		if _, err := s.opts.Runner.CloseConversation(st.ID); err != nil {
			return State{}, err
		}
		delete(s.state.Stores, store)
		if err := s.save(); err != nil {
			return State{}, err
		}
	}
	return s.open(store, root, s.selection(hs), hs)
}

type selectRequest struct {
	Harness *string `json:"harness"`
	Model   *string `json:"model"`
	Effort  *string `json:"effort"`
}

// Select saves a selection as the machine-wide last choice and applies it
// to the store's open conversation: a harness change replaces the
// conversation with one on the new harness's profile (model and effort
// reset unless given), and a model or effort change applies from its next
// message. With no open conversation only the choice is saved.
func (s *Support) Select(store string, req selectRequest) (SelectAnswer, error) {
	root, err := s.root(store)
	if err != nil {
		return SelectAnswer{}, err
	}
	if req.Model != nil && *req.Model != "" && !modelPattern.MatchString(*req.Model) {
		return SelectAnswer{}, badRequest("model %q must match %s", *req.Model, modelPattern.String())
	}
	if req.Effort != nil && *req.Effort != "" && !effortPattern.MatchString(*req.Effort) {
		return SelectAnswer{}, badRequest("effort %q must be a lowercase word", *req.Effort)
	}
	hs := s.harnesses()
	s.mu.Lock()
	defer s.mu.Unlock()
	// The base is the open conversation's own settings, else the last
	// selection: "same harness" is the conversation's.
	st, open := s.current(store, root)
	cur := s.selection(hs)
	if open {
		cur = Selection{Harness: st.Backend, Model: st.Model, Effort: st.Effort}
	}
	next := cur
	if req.Harness != nil {
		h, ok := findHarness(hs, *req.Harness)
		switch {
		case !ok:
			return SelectAnswer{}, badRequest("unknown support harness %q", *req.Harness)
		case !h.Available:
			return SelectAnswer{}, badRequest("support harness %q is not available: %s", h.Harness, h.Error)
		}
		if h.Harness != cur.Harness {
			next = Selection{Harness: h.Harness}
		}
	}
	if req.Model != nil {
		next.Model = *req.Model
	}
	if req.Effort != nil {
		next.Effort = *req.Effort
	}
	ans := SelectAnswer{Selection: next}
	switch {
	case !open:
	case next.Harness != cur.Harness:
		fresh, err := s.open(store, root, next, hs)
		if err != nil {
			return SelectAnswer{}, err
		}
		if _, err := s.opts.Runner.CloseConversation(st.ID); err != nil {
			return SelectAnswer{}, err
		}
		ans.Conversation = &fresh
	case req.Model != nil || req.Effort != nil:
		changed, err := s.opts.Runner.SetSettings(st.ID, req.Model, req.Effort)
		if err != nil {
			return SelectAnswer{}, err
		}
		ans.Conversation = &changed
	default:
		ans.Conversation = &st
	}
	s.state.Last = &next
	if err := s.save(); err != nil {
		return SelectAnswer{}, err
	}
	return ans, nil
}

// root returns the store's working root; an unknown store is 404.
func (s *Support) root(store string) (string, error) {
	root, err := s.opts.StoreRoot(store)
	if err != nil {
		return "", &requestError{status: http.StatusNotFound, msg: fmt.Sprintf("unknown tracker store %q: %v", store, err)}
	}
	return root, nil
}

// ServeHTTP serves SupportRoutePrefix and everything under it.
func (s *Support) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(req.URL.Path, SupportRoutePrefix), "/")
	parts := strings.Split(rest, "/")
	if req.Method == http.MethodPost && !writeAllowed(req) {
		http.Error(w, "support routes take a JSON body from this machine's clients or the dashboard's own pages", http.StatusForbidden)
		return
	}
	switch {
	case len(parts) == 1 && parts[0] == "choices" && req.Method == http.MethodGet:
		writeAnswer(w, http.StatusOK, s.Choices(req.Context()), nil)
	case len(parts) < 2 || len(parts) > 3 || parts[0] != "stores" || parts[1] == "":
		http.Error(w, "unknown support route", http.StatusNotFound)
	case len(parts) == 2 && req.Method == http.MethodGet:
		ans, err := s.Store(parts[1])
		writeAnswer(w, http.StatusOK, ans, err)
	case len(parts) == 2 || req.Method != http.MethodPost:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	case parts[2] == "send":
		var body sendRequest
		if !decode(w, req, &body) {
			return
		}
		st, err := s.Send(parts[1], body.Text)
		writeAnswer(w, http.StatusOK, st, err)
	case parts[2] == "new":
		var body struct{}
		if !decode(w, req, &body) {
			return
		}
		st, err := s.New(parts[1])
		writeAnswer(w, http.StatusOK, st, err)
	case parts[2] == "select":
		var body selectRequest
		if !decode(w, req, &body) {
			return
		}
		ans, err := s.Select(parts[1], body)
		writeAnswer(w, http.StatusOK, ans, err)
	default:
		http.Error(w, "unknown support route", http.StatusNotFound)
	}
}
