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
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/rock3r/cavalry-skill/internal/config"
)

// Request is what the bridge receives.
type Request struct {
	ID             string `json:"id"`
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
}

type LogLine struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type ScriptError struct {
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
	CavalryVersion string `json:"cavalryVersion"`
}

// State of a job while the client waits for it.
type State string

const (
	StateQueued  State = "queued"
	StateRunning State = "running"
	StateDone    State = "done"
	// StateBusy means Cavalry has not answered for a while; the job is probably still working.
	StateBusy State = "busy"
)

var (
	// ErrUnavailable means no bridge answered at all.
	ErrUnavailable = errors.New("cannot reach cav-bridge")
	// ErrStillRunning means the wait timed out while the job was queued or running.
	ErrStillRunning = errors.New("job still running")
	// ErrLost means the bridge stopped answering while the job was in flight.
	ErrLost = errors.New("bridge stopped answering while the job was in flight")
)

const unavailableHint = "Start Cavalry, then run Scripts > cav-bridge and keep its window open. `cav doctor` checks every step."

type Client struct {
	Host  string
	Port  int
	Spool string
	http  *http.Client
}

func New() *Client {
	return &Client{
		Host:  config.Host(),
		Port:  config.Port(),
		Spool: spoolUnlessRelayed(),
		http:  &http.Client{Timeout: 5 * time.Second},
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
		return nil, fmt.Errorf("%w at %s: %s", ErrUnavailable, c.base(), unavailableHint)
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
	body, _ := json.Marshal(r)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/post", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w at %s: %s", ErrUnavailable, c.base(), unavailableHint)
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
	if c.Spool != "" {
		return c.waitSpool(ctx, id, timeout, onState)
	}
	deadline := time.Now().Add(timeout)
	state := StateQueued
	set := func(s State) {
		if s != state {
			state = s
			if onState != nil {
				onState(s)
			}
		}
	}
	jobFile := filepath.Join(config.JobsDir(), id+".json")
	var lastSeen = time.Now()
	warnedBusy := false
	for i := 0; ; i++ {
		if r, ok := readResultFile(jobFile); ok {
			set(StateDone)
			return r, nil
		}
		payload, err := c.getRaw(ctx)
		if err == nil {
			lastSeen = time.Now()
			var head struct {
				Type string `json:"type"`
				ID   string `json:"id"`
			}
			_ = json.Unmarshal(payload, &head)
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
		} else if time.Since(lastSeen) > 120*time.Second {
			// Cavalry cannot answer while a native operation blocks it (for example deleting
			// hundreds of layers), so only give up after a long silence.
			return nil, fmt.Errorf("%w (job %s). Cavalry may have crashed or the bridge window was closed. %s", ErrLost, id, unavailableHint)
		} else if time.Since(lastSeen) > 10*time.Second && onState != nil && !warnedBusy {
			warnedBusy = true
			onState(StateBusy)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: job %s is %s after %s", ErrStillRunning, id, state, timeout)
		}
		select {
		case <-ctx.Done():
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

func (c *Client) waitSpool(ctx context.Context, id string, timeout time.Duration, onState func(State)) (*Result, error) {
	deadline := time.Now().Add(timeout)
	resPath := filepath.Join(c.Spool, id+".res.json")
	statePath := filepath.Join(c.Spool, id+".state")
	state := StateQueued
	for i := 0; ; i++ {
		if r, ok := readResultFile(resPath); ok {
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
	_ = os.Remove(filepath.Join(config.JobsDir(), id+".json"))
	_ = os.Remove(filepath.Join(config.JobsDir(), id+".js"))
}
