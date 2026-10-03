// Package operation stores command checkpoints separately from bridge jobs.
package operation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/rock3r/cav/internal/bridge"
)

type Job struct {
	ID            string         `json:"id"`
	Phase         string         `json:"phase"`
	Code          string         `json:"code,omitempty"`
	HelperVersion string         `json:"helperVersion,omitempty"`
	Helpers       bool           `json:"helpers"`
	Submission    string         `json:"submission"` // prepared, uncertain, accepted
	State         bridge.State   `json:"state"`
	Result        *bridge.Result `json:"result,omitempty"`
}
type Render struct {
	Stage          string  `json:"staging"`
	ExpectedFrames int     `json:"expectedFrames"`
	FPS            float64 `json:"fps"`
}
type Record struct {
	Schema          int               `json:"schema"`
	ID              string            `json:"id"`
	Command         []string          `json:"command"`
	Cwd             string            `json:"cwd"`
	Host            string            `json:"host"`
	Port            int               `json:"port"`
	Session         string            `json:"bridgeSession,omitempty"`
	Spool           string            `json:"spool,omitempty"`
	Phase           string            `json:"phase"`
	Status          string            `json:"status"`
	Updated         time.Time         `json:"updated"`
	Jobs            []*Job            `json:"jobs"`
	OutputDir       string            `json:"defaultOutputDir,omitempty"`
	IntendedOutputs []string          `json:"intendedOutputs,omitempty"`
	Inputs          map[string]string `json:"inputHashes,omitempty"`
	Outputs         []string          `json:"outputs,omitempty"`
	Data            json.RawMessage   `json:"data,omitempty"`
	Completed       map[string]bool   `json:"completed,omitempty"`
	Progress        string            `json:"progress,omitempty"`
	Error           string            `json:"error,omitempty"`
	FailureReason   string            `json:"failureReason,omitempty"`
	Render          *Render           `json:"render,omitempty"`
}

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func Path(home, id string) (string, error) {
	if !validID.MatchString(id) {
		return "", fmt.Errorf("invalid operation id")
	}
	return filepath.Join(home, "operations", id+".json"), nil
}
func Load(home, id string) (*Record, error) {
	p, err := Path(home, id)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var r Record
	if err = json.Unmarshal(b, &r); err != nil {
		return nil, err
	}
	if r.Schema != 1 || r.ID != id {
		return nil, fmt.Errorf("unsupported or corrupt operation record")
	}
	for _, j := range r.Jobs {
		if j == nil || !validID.MatchString(j.ID) {
			return nil, fmt.Errorf("corrupt job checkpoint")
		}
		if j.Result != nil && j.Result.ID != j.ID {
			return nil, fmt.Errorf("checkpoint result id differs from job id")
		}
	}
	return &r, nil
}

// Save uses a unique temporary file and fsync before atomic publication.
func Save(home string, r *Record) error {
	p, err := Path(home, r.ID)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	r.Updated = time.Now().UTC()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".checkpoint-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p)
}

// Lock is kernel-owned. A crashed process releases it; no unsafe stale-lock stealing.
func Lock(home string) (func(), error) {
	if err := os.MkdirAll(filepath.Join(home, "operations"), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(home, "operations", "client.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("another operation client is active: %w", err)
	}
	return func() { f.Close() }, nil
}

// View omits replay source, keeping status output small and command-focused.
func View(r *Record) *Record {
	copy := *r
	copy.Jobs = make([]*Job, len(r.Jobs))
	for i, j := range r.Jobs {
		v := *j
		v.Code = ""
		copy.Jobs[i] = &v
	}
	return &copy
}
