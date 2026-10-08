// Package bridge talks to cav-bridge.js, the UI script that runs inside Cavalry.
//
// A job is a piece of JavaScript. The client hands it to the bridge, then waits for
// the result. Two transports exist:
//
//   - HTTP (default): POST /post, then poll GET /get. The bridge answers GET while a
//     job runs (verified on Cavalry 2.7.2), so the client can tell "running" from "lost".
//   - Spool (CAV_SPOOL=dir): the client writes <id>.req.json into dir and waits for
//     <id>.res.json. `cav relay` forwards spool files to the bridge over HTTP. This lets
//     an agent that runs in a sandbox without loopback access still drive Cavalry.
package bridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rock3r/cav/internal/cavapp"
	"github.com/rock3r/cav/internal/config"
)

// Protocol is the version of the request and result format that this cav speaks. Change it
// only when that format changes; the bridge refuses requests with another protocol.
const Protocol = 1

// Request is what the bridge receives.
type Request struct {
	ID             string `json:"id"`
	Protocol       int    `json:"protocol"`
	Token          string `json:"token,omitempty"`
	Code           string `json:"code,omitempty"`
	File           string `json:"file,omitempty"`
	Preload        string `json:"preload,omitempty"`
	PreloadVersion string `json:"preloadVersion,omitempty"`
	// Restricted asks the bridge to disable process-launching APIs during the job.
	Restricted bool `json:"restricted,omitempty"`
}

// Result is what the bridge returns for one job.
type Result struct {
	Type          string          `json:"type"`
	ID            string          `json:"id"`
	OK            bool            `json:"ok"`
	Value         json.RawMessage `json:"value"`
	Logs          []LogLine       `json:"logs"`
	Error         *ScriptError    `json:"error"`
	MS            int64           `json:"ms"`
	BridgeVersion string          `json:"bridgeVersion,omitempty"`
	BridgeSession string          `json:"bridgeSession,omitempty"`
	Protocol      int             `json:"protocol,omitempty"`
}

type LogLine struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type ScriptError struct {
	Code    string `json:"code,omitempty"` // "protocol" when the bridge refused the request format
	Message string `json:"message"`
	Stack   string `json:"stack,omitempty"`
	Where   *struct {
		Line   int `json:"line"`
		Column int `json:"column"`
	} `json:"where,omitempty"`
}

// Hello is the payload the bridge publishes before its first job.
type Hello struct {
	Type           string `json:"type"`
	ID             string `json:"id"`
	Bridge         string `json:"bridge"`
	BridgeVersion  string `json:"bridgeVersion"`
	Protocol       int    `json:"protocol,omitempty"`
	CavalryVersion string `json:"cavalryVersion"`
}

// State of a job while the client waits for it.
type State string

const (
	StateQueued  State = "queued"
	StateRunning State = "running"
	StateDone    State = "done"
	StateUnknown State = "unknown"
	// StateBusy means Cavalry has not answered for a while; the job is probably still working.
	StateBusy State = "busy"
)

var (
	// ErrRefused means nothing listens on the bridge port, so the bridge is not running.
	ErrRefused = errors.New("connection refused")
	// ErrUnavailable means no bridge answered at all.
	ErrUnavailable = errors.New("cannot reach cav-bridge")
	// ErrStillRunning means the wait timed out while the job was queued or running.
	ErrStillRunning = errors.New("job still running")
	// ErrUnknownJob means no job with that id is queued, running or stored.
	ErrUnknownJob = errors.New("unknown job")
	// ErrBlocked means a sandbox around cav refused the connection or a job-file write.
	ErrBlocked = errors.New("a sandbox blocked cav")
	// ErrProtocol means the running bridge speaks another request format than this cav.
	ErrProtocol = errors.New("cav and cav-bridge do not match")
	// ErrLost means the bridge stopped answering while the job was in flight.
	ErrLost           = errors.New("bridge stopped answering while the job was in flight")
	ErrDisconnected   = errors.New("bridge connection refused before job completion")
	ErrSessionChanged = errors.New("bridge session changed before job completion")
)

// Diagnose checks the Cavalry process for a bridge on host. "Cavalry is not running" is
// always safe to report. "Cavalry is running but cav-bridge is not" is reported only when
// the connection was refused, which proves that nothing listens: a silent bridge may
// only be busy with a long native call.
func Diagnose(host string, refused bool) cavapp.Diagnosis {
	unknown := cavapp.Explain(cavapp.Unknown, nil)
	if !IsLocal(host) {
		return unknown
	}
	d := cavapp.Diagnose()
	if d.Status == cavapp.Running && !refused {
		return unknown
	}
	return d
}

// unavailableHint says what to do when no bridge answers.
func unavailableHint(host string, refused bool) string {
	return Diagnose(host, refused).Hint() + " `cav doctor` checks every step."
}

// exitedWhat says what stopped when the bridge refuses a connection mid-job.
func exitedWhat(host string) string {
	if !IsLocal(host) {
		return "Cavalry or its bridge may have exited"
	}
	d := Diagnose(host, true)
	if d.What == "" {
		return "Cavalry or its bridge may have exited"
	}
	if note := d.CrashNote(); note != "" {
		return d.What + ". " + strings.TrimSuffix(note, ".")
	}
	return d.What
}

// IsLocal reports whether host names this machine, where cav can look at the processes.
func IsLocal(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// slowHint explains a bridge that accepts the connection but answers late. Cavalry serves
// requests on its UI thread, so a long script, a render or an open dialog box delays
// the answer.
const slowHint = "Cavalry accepted the connection but did not answer in time. It may be busy (a long script, a render or an open dialog box); inspect existing status and wait; do not resubmit work whose delivery is uncertain."

// answerTimeout is how long cav waits for the bridge to answer one request. It is long
// because a busy Cavalry answers late; a stopped bridge refuses at once.
const answerTimeout = 30 * time.Second

// SandboxHint says what to do when an agent's sandbox blocks cav.
const SandboxHint = "cav runs inside a sandbox (for example an agent's) that blocks connections to 127.0.0.1 or writes to ~/.cav. " +
	"Either let cav connect to 127.0.0.1:8723 and write ~/.cav, or use spool mode: run `cav relay --spool <folder in the project>` " +
	"outside the sandbox, and put that folder's path in a `.cav-spool` file in the project (see `cav help relay`)"

func unreachable(host, base string, err error) error {
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return fmt.Errorf("%w: %w: connecting to %s was not permitted. %s", ErrUnavailable, ErrBlocked, base, SandboxHint)
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("%w at %s: %w. %s", ErrUnavailable, base, ErrRefused, unavailableHint(host, true))
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return fmt.Errorf("%w at %s: %w (no answer before the request deadline). %s", ErrUnavailable, base, err, slowHint)
	}
	return fmt.Errorf("%w at %s: %v. %s", ErrUnavailable, base, err, unavailableHint(host, false))
}

type Client struct {
	Host            string
	Port            int
	Spool           string
	ExpectedSession string // optional operation identity; older bridges omit this
	http            *http.Client
}

func New() *Client {
	return &Client{
		Host:  config.Host(),
		Port:  config.Port(),
		Spool: spoolUnlessRelayed(),
		http:  &http.Client{Timeout: answerTimeout},
	}
}

// A process started by `cav relay` must talk HTTP even when a spool marker is present.
func spoolUnlessRelayed() string {
	if os.Getenv("CAV_RELAYED") == "1" {
		return ""
	}
	return config.SpoolDir()
}

func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}

func (c *Client) base() string { return fmt.Sprintf("http://%s:%d", c.Host, c.Port) }

// Probe returns the current GET payload, or ErrUnavailable.
func (c *Client) Probe(ctx context.Context) (map[string]any, error) {
	if c.Spool != "" {
		return nil, errors.New("probe is not available through a spool; run a job instead")
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/get", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, unreachable(c.Host, c.base(), err)
	}
	defer resp.Body.Close()
	var payload map[string]any
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("unexpected reply from %s (is another program on port %d?)", c.base(), c.Port)
	}
	return payload, nil
}

// Submit sends a job. The caller fills Code or File; Submit fills ID and Token.
func (c *Client) Submit(ctx context.Context, r *Request) error {
	if r.ID == "" {
		r.ID = NewID()
	}
	if c.Spool != "" {
		return writeJSONAtomic(filepath.Join(c.Spool, r.ID+".req.json"), r)
	}
	tok, err := config.ReadToken()
	if err != nil {
		return err
	}
	r.Token = tok
	r.Protocol = Protocol
	body, _ := json.Marshal(r)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/post", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return unreachable(c.Host, c.base(), err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("bridge answered POST with HTTP %d", resp.StatusCode)
	}
	return nil
}

// Wait blocks until the job finishes, the timeout passes (ErrStillRunning), or the
// bridge disappears (ErrLost). onState is called when the state changes.
func (c *Client) Wait(ctx context.Context, id string, timeout time.Duration, onState func(State)) (*Result, error) {
	if !validJobID(id) {
		return nil, fmt.Errorf("invalid job id")
	}
	r, err := c.wait(ctx, id, timeout, StateUnknown, onState)
	if err == nil {
		if perr := ProtocolError(r); perr != nil {
			return nil, perr
		}
	}
	return r, err
}

// ProtocolError reports a result from a bridge that speaks another protocol. A bridge older
// than cav 1.0 sends no protocol; it speaks protocol 1.
func ProtocolError(r *Result) error {
	got := r.Protocol
	if r.Error != nil && r.Error.Code == "protocol" {
		return fmt.Errorf("%w: %s. %s", ErrProtocol, r.Error.Message, protocolHint)
	}
	if got != 0 && got != Protocol {
		return fmt.Errorf("%w: cav-bridge %s speaks protocol %d, this cav speaks protocol %d. %s", ErrProtocol, r.BridgeVersion, got, Protocol, protocolHint)
	}
	return nil
}

const protocolHint = "Run `cav setup` to install the matching bridge script, then close the cav-bridge window in Cavalry and start it again from the Scripts menu"

func (c *Client) wait(ctx context.Context, id string, timeout time.Duration, initial State, onState func(State)) (*Result, error) {
	if c.Spool != "" {
		return c.waitSpool(ctx, id, timeout, initial, onState)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	deadline := time.Now().Add(timeout)
	state := initial
	set := func(s State) {
		if s != state {
			state = s
			if onState != nil {
				onState(s)
			}
		}
	}
	jobFile := filepath.Join(config.JobsDir(), id+".json")
	scriptFile := filepath.Join(config.JobsDir(), id+".js")
	var lastSeen = time.Now()
	var idleSince time.Time
	warnedBusy := false
	for i := 0; ; i++ {
		if r, ok := readResultFile(jobFile); ok && r.ID == id {
			if err := c.checkSession(r.BridgeSession, id); err != nil {
				set(StateUnknown)
				return nil, err
			}
			set(StateDone)
			return r, nil
		}
		payload, err := c.getRaw(ctx)
		if err == nil {
			lastSeen = time.Now()
			var head struct {
				Type    string `json:"type"`
				ID      string `json:"id"`
				Session string `json:"bridgeSession"`
			}
			_ = json.Unmarshal(payload, &head)
			if err := c.checkSession(head.Session, id); err != nil {
				set(StateUnknown)
				return nil, err
			}
			// A job that is not running, has no script waiting and no stored result is unknown
			// (a wrong id, or a result pruned after a day). Say so instead of waiting forever.
			if head.ID != id && head.Type != "running" && !exists(scriptFile) && !exists(jobFile) {
				if idleSince.IsZero() {
					idleSince = time.Now()
				} else if time.Since(idleSince) > 5*time.Second {
					return nil, fmt.Errorf("%w: job %s is not running and has no stored result (check the id; results are kept for a day)", ErrUnknownJob, id)
				}
			} else {
				idleSince = time.Time{}
			}
			if head.ID == id {
				switch head.Type {
				case "running":
					set(StateRunning)
				case "result":
					var r Result
					if err := json.Unmarshal(payload, &r); err == nil {
						set(StateDone)
						return &r, nil
					}
				}
			}
		} else if errors.Is(err, syscall.ECONNREFUSED) {
			// Refusal proves the listener is absent, unlike a silent native call. It does
			// not prove a crash or lost output: check a result that may have arrived last.
			if r, ok := readResultFile(jobFile); ok && r.ID == id {
				if e := c.checkSession(r.BridgeSession, id); e != nil {
					set(StateUnknown)
					return nil, e
				}
				set(StateDone)
				return r, nil
			}
			set(StateUnknown)
			return nil, fmt.Errorf("%w: %w (job %s). %s; the native outcome is unknown. Preserve partial output and inspect the operation before retrying.", ErrLost, ErrDisconnected, id, exitedWhat(c.Host))
		} else if time.Since(lastSeen) > 120*time.Second {
			// Cavalry cannot answer while a native operation blocks it (for example deleting
			// hundreds of layers), so only give up after a long silence.
			return nil, fmt.Errorf("%w (job %s). Its state is unknown: a blocking native call, closed bridge or crashed app can all cause this silence. %s", ErrLost, id, unavailableHint(c.Host, false))
		} else if time.Since(lastSeen) > 10*time.Second && onState != nil && !warnedBusy {
			warnedBusy = true
			set(StateBusy)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: job %s is %s after %s", ErrStillRunning, id, state, timeout)
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: job %s is %s after %s", ErrStillRunning, id, state, timeout)
			}
			return nil, ctx.Err()
		case <-time.After(pollDelay(i)):
		}
	}
}

func pollDelay(i int) time.Duration {
	if i < 40 {
		return 50 * time.Millisecond
	}
	return 250 * time.Millisecond
}

func (c *Client) getRaw(ctx context.Context) ([]byte, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/get", nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (c *Client) waitSpool(ctx context.Context, id string, timeout time.Duration, initial State, onState func(State)) (*Result, error) {
	deadline := time.Now().Add(timeout)
	resPath := filepath.Join(c.Spool, id+".res.json")
	statePath := filepath.Join(c.Spool, id+".state")
	state := initial
	for i := 0; ; i++ {
		if r, ok := readResultFile(resPath); ok && r.ID == id {
			if e := c.checkSession(r.BridgeSession, id); e != nil {
				return nil, e
			}
			if onState != nil {
				onState(StateDone)
			}
			return r, nil
		}
		if b, err := os.ReadFile(statePath); err == nil {
			s := State(bytes.TrimSpace(b))
			if s != state {
				state = s
				if onState != nil {
					onState(s)
				}
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: job %s is %s after %s (spool %s; is `cav relay` running?)", ErrStillRunning, id, state, timeout, c.Spool)
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("%w: job %s is %s after %s", ErrStillRunning, id, state, timeout)
			}
			return nil, ctx.Err()
		case <-time.After(pollDelay(i)):
		}
	}
}

func readResultFile(path string) (*Result, bool) {
	b, err := os.ReadFile(path)
	if err != nil || len(b) == 0 {
		return nil, false
	}
	var r Result
	if err := json.Unmarshal(b, &r); err != nil || r.Type != "result" {
		return nil, false
	}
	return &r, true
}

func writeJSONAtomic(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Cleanup removes the bridge's copy of a finished job's result.
func Cleanup(id string) {
	if os.Getenv("CAV_KEEP_JOBS") != "" {
		return
	}
	// The result stays, so a later `cav job wait <id>` still finds it; only the script goes.
	// Results older than a day are pruned.
	_ = os.Remove(filepath.Join(config.JobsDir(), id+".js"))
	old, _ := filepath.Glob(filepath.Join(config.JobsDir(), "*.json"))
	for _, f := range old {
		if st, err := os.Stat(f); err == nil && time.Since(st.ModTime()) > 24*time.Hour {
			_ = os.Remove(f)
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Inspect reads existing state without submitting code. A silent native call is busy,
// not proof of a crash. Old bridges expose only the current job; others are unknown.
func (c *Client) Inspect(ctx context.Context, id string) (State, *Result, error) {
	if !validJobID(id) {
		return StateUnknown, nil, fmt.Errorf("invalid job id")
	}
	dir := config.JobsDir()
	suffix := ".json"
	if c.Spool != "" {
		dir = c.Spool
		suffix = ".res.json"
	}
	if r, ok := readResultFile(filepath.Join(dir, id+suffix)); ok && r.ID == id {
		if e := c.checkSession(r.BridgeSession, id); e != nil {
			return StateUnknown, nil, e
		}
		return StateDone, r, ProtocolError(r)
	}
	if c.Spool != "" {
		if b, e := os.ReadFile(filepath.Join(dir, id+".state")); e == nil {
			s := State(bytes.TrimSpace(b))
			if s == StateRunning || s == StateQueued || s == StateBusy {
				return s, nil, nil
			}
		}
		return StateUnknown, nil, nil
	}
	b, e := c.getRaw(ctx)
	if e != nil {
		if errors.Is(e, syscall.ECONNREFUSED) {
			return StateUnknown, nil, fmt.Errorf("%w: %w (job %s)", ErrLost, ErrDisconnected, id)
		}
		return ConnectionState(e), nil, e
	}
	var head struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Session string `json:"bridgeSession"`
	}
	if e = json.Unmarshal(b, &head); e != nil {
		return StateUnknown, nil, e
	}
	if e = c.checkSession(head.Session, id); e != nil {
		return StateUnknown, nil, e
	}
	if head.ID == id {
		if head.Type == "running" {
			return StateRunning, nil, nil
		}
		if head.Type == "result" {
			var r Result
			if e = json.Unmarshal(b, &r); e != nil {
				return StateUnknown, nil, e
			}
			return StateDone, &r, ProtocolError(&r)
		}
	}
	return StateUnknown, nil, nil
}
func validJobID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// SubmitDeadline uses the complete operation budget instead of a shorter HTTP timeout.
// Ambiguous delivery must be checkpointed by the caller before invoking this method.
func (c *Client) SubmitDeadline(ctx context.Context, r *Request) error {
	clone := *c
	clone.http = &http.Client{}
	return clone.Submit(ctx, r)
}

// WaitAccepted retains known queue acceptance while waiting for an operation job.
func (c *Client) WaitAccepted(ctx context.Context, id string, timeout time.Duration, state State, onState func(State)) (*Result, error) {
	if !validJobID(id) {
		return nil, fmt.Errorf("invalid job id")
	}
	r, e := c.wait(ctx, id, timeout, state, onState)
	if e == nil {
		e = ProtocolError(r)
	}
	return r, e
}
func ConnectionState(err error) State {
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
		return StateBusy
	}
	return StateUnknown
}

func (c *Client) checkSession(session, id string) error {
	if c.ExpectedSession != "" && session != "" && c.ExpectedSession != session {
		return fmt.Errorf("%w: %w (job %s); a new bridge cannot finish the original native call; preserve outputs and inspect the operation", ErrLost, ErrSessionChanged, id)
	}
	return nil
}
