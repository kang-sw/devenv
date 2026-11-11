package wsagent

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// RoutePrefix is where the daemon mounts the runner's routes:
//
//	GET  /agents                  every conversation's state
//	POST /agents                  {profile, root} opens a conversation
//	GET  /agents/<id>             one conversation's state
//	POST /agents/<id>/send        {text} holds a message for the next turn
//	POST /agents/<id>/interrupt   stops the running turn; returns held text
//	POST /agents/<id>/stop        ends the process (resumes on next send)
//	POST /agents/<id>/close       ends the conversation for good
//	POST /agents/<id>/settings    {model?, effort?} for the next turn
//
// and, for a hosted lead session:
//
//	POST /agents/leads            {root, harness, model?, effort?,
//	                              permission_mode?} spawns one
//	POST /agents/<id>/settings    also takes permission_mode
//	POST /agents/<id>/approvals/<request-id>
//	                              {behavior: "allow"|"deny", message?}
//	                              answers a waiting tool permission prompt
//
// The routes sit behind the daemon's access guard. A POST must carry a JSON
// body (Content-Type application/json) and either no Origin (a non-browser
// client) or the daemon's own origin, so no other web page can drive an
// agent: a browser always sends Origin on a cross-origin POST, and the JSON
// content type forces a CORS preflight the daemon never answers.
//
// A lead session runs with the user's own tools and permissions, so the
// writes that decide what it may do take the stricter trusted gate
// (Options.Trusted) on top: its spawn, a settings change, stop, close, and
// an approval answer are answered 404 to anyone but a browser on this
// machine, and 403 without the daemon's own Origin. A LAN viewer with the
// token reads a lead session and may send and interrupt, as in any
// session; State.CanControl tells a page which viewer it serves.
const RoutePrefix = "/agents"

// leadsRoute is the id segment the lead spawn route takes; no conversation
// has it (an id is a reply-id).
const leadsRoute = "leads"

// requestBodyLimit bounds a route's request body.
const requestBodyLimit = 256 << 10

type createRequest struct {
	Profile string `json:"profile"`
	Root    string `json:"root"`
}

type sendRequest struct {
	Text string `json:"text"`
}

type settingsRequest struct {
	Model  *string `json:"model"`
	Effort *string `json:"effort"`
	// PermissionMode is a claude lead session's only.
	PermissionMode *string `json:"permission_mode"`
}

type leadRequest struct {
	Root           string `json:"root"`
	Harness        string `json:"harness"`
	Model          string `json:"model"`
	Effort         string `json:"effort"`
	PermissionMode string `json:"permission_mode"`
}

type approvalRequest struct {
	Behavior string `json:"behavior"`
	Message  string `json:"message"`
}

type interruptAnswer struct {
	State
	// Returned is the text of each held message the interrupt took back.
	Returned []string `json:"returned"`
}

// Handler serves RoutePrefix and everything under it.
func (r *Runner) Handler() http.Handler {
	return http.HandlerFunc(r.serve)
}

func (r *Runner) serve(w http.ResponseWriter, req *http.Request) {
	rest := strings.TrimPrefix(req.URL.Path, RoutePrefix)
	rest = strings.Trim(rest, "/")
	var id, action string
	if rest != "" {
		id, action, _ = strings.Cut(rest, "/")
	}
	if req.Method == http.MethodPost && !writeAllowed(req) {
		http.Error(w, "agent routes take a JSON body from this machine's clients or the dashboard's own pages", http.StatusForbidden)
		return
	}
	local, own := r.opts.Trusted(req)
	// view marks each state with whether this viewer holds the lead
	// controls.
	view := func(st State) State {
		st.CanControl = st.Lead && local
		return st
	}
	if req.Method == http.MethodPost && (id == leadsRoute || (r.IsLead(id) && leadControl(action))) {
		if !local {
			http.Error(w, "unknown agent route", http.StatusNotFound)
			return
		}
		if !own {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
	}
	switch {
	case id == "" && req.Method == http.MethodGet:
		list := r.List()
		for i := range list {
			list[i] = view(list[i])
		}
		writeAnswer(w, http.StatusOK, map[string]any{"conversations": list}, nil)
	case id == leadsRoute:
		if req.Method != http.MethodPost || action != "" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body leadRequest
		if !decode(w, req, &body) {
			return
		}
		st, err := r.CreateLead(body.Harness, body.Root, LeadOptions{Model: body.Model, Effort: body.Effort, PermissionMode: body.PermissionMode})
		writeAnswer(w, http.StatusCreated, view(st), err)
	case id == "" && req.Method == http.MethodPost:
		var body createRequest
		if !decode(w, req, &body) {
			return
		}
		st, err := r.Create(body.Profile, body.Root)
		writeAnswer(w, http.StatusCreated, st, err)
	case action == "" && req.Method == http.MethodGet:
		st, err := r.Get(id)
		writeAnswer(w, http.StatusOK, view(st), err)
	case req.Method != http.MethodPost || action == "":
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	case action == "send":
		var body sendRequest
		if !decode(w, req, &body) {
			return
		}
		st, err := r.Send(id, body.Text)
		writeAnswer(w, http.StatusOK, view(st), err)
	case action == "interrupt":
		st, returned, err := r.Interrupt(id)
		if returned == nil {
			returned = []string{}
		}
		writeAnswer(w, http.StatusOK, interruptAnswer{State: view(st), Returned: returned}, err)
	case action == "stop":
		st, err := r.Stop(id)
		writeAnswer(w, http.StatusOK, view(st), err)
	case action == "close":
		st, err := r.CloseConversation(id)
		writeAnswer(w, http.StatusOK, view(st), err)
	case action == "settings":
		var body settingsRequest
		if !decode(w, req, &body) {
			return
		}
		st, err := r.SetSettings(id, body.Model, body.Effort)
		if err == nil && body.PermissionMode != nil {
			st, err = r.SetPermissionMode(id, *body.PermissionMode)
		}
		writeAnswer(w, http.StatusOK, view(st), err)
	case strings.HasPrefix(action, "approvals/"):
		var body approvalRequest
		if !decode(w, req, &body) {
			return
		}
		if body.Behavior != "allow" && body.Behavior != "deny" {
			writeAnswer(w, 0, nil, badRequest("behavior must be \"allow\" or \"deny\"; got %q", body.Behavior))
			return
		}
		st, err := r.AnswerApproval(id, strings.TrimPrefix(action, "approvals/"), body.Behavior == "allow", body.Message)
		writeAnswer(w, http.StatusOK, view(st), err)
	default:
		http.Error(w, "unknown agent route", http.StatusNotFound)
	}
}

// leadControl reports whether action is one of a lead session's control
// writes, which take the trusted gate; send and interrupt do not.
func leadControl(action string) bool {
	return action == "stop" || action == "close" || action == "settings" || strings.HasPrefix(action, "approvals/")
}

// writeAllowed is the POST rule: a JSON body, and no Origin or the
// daemon's own.
func writeAllowed(req *http.Request) bool {
	mt, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		return false
	}
	origin, ok := req.Header["Origin"]
	if !ok {
		return true
	}
	if len(origin) != 1 || origin[0] == "" || origin[0] == "null" {
		return false
	}
	u, err := url.Parse(origin[0])
	if err != nil || u.Scheme != "http" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" {
		return false
	}
	return strings.EqualFold(u.Host, req.Host)
}

// decode reads a JSON body into v; an empty body is an empty object.
func decode(w http.ResponseWriter, req *http.Request, v any) bool {
	raw, err := io.ReadAll(io.LimitReader(req.Body, requestBodyLimit+1))
	if err != nil {
		http.Error(w, "request body unreadable", http.StatusBadRequest)
		return false
	}
	if len(raw) > requestBodyLimit {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return false
	}
	if strings.TrimSpace(string(raw)) == "" {
		return true
	}
	if err := json.Unmarshal(raw, v); err != nil {
		http.Error(w, "malformed JSON body: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeAnswer(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		var re *requestError
		if errors.As(err, &re) {
			http.Error(w, re.msg, re.status)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
