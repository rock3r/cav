package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/rock3r/cav/internal/config"
)

type fixtureTransport func(*http.Request) (*http.Response, error)

func (f fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func clientFixture(t *testing.T) *Client {
	t.Helper()
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_RELAYED", "1")
	if err := os.MkdirAll(config.JobsDir(), 0700); err != nil {
		t.Fatal(err)
	}
	return New()
}
func fixtureReply(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
func refused() error { return &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED} }

func TestRefusedBridgeStopsWaitingWithoutClaimingCrash(t *testing.T) {
	c := clientFixture(t)
	calls := 0
	c.http.Transport = fixtureTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, refused() })
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	state := StateQueued
	_, err := c.WaitAccepted(ctx, "render-job", time.Hour, state, func(s State) { state = s })
	if !errors.Is(err, ErrDisconnected) || !errors.Is(err, ErrLost) || state != StateUnknown || calls != 1 {
		t.Fatalf("err=%v state=%s calls=%d", err, state, calls)
	}
}
func TestRestartedBridgeStopsWaitingForOldSession(t *testing.T) {
	c := clientFixture(t)
	c.ExpectedSession = "original"
	// A leftover script must not keep a new bridge waiting for work it cannot finish.
	if err := os.WriteFile(filepath.Join(config.JobsDir(), "old-job.js"), []byte("native render"), 0600); err != nil {
		t.Fatal(err)
	}
	c.http.Transport = fixtureTransport(func(*http.Request) (*http.Response, error) {
		return fixtureReply(`{"type":"hello","bridgeSession":"restarted"}`), nil
	})
	state := StateQueued
	_, err := c.WaitAccepted(t.Context(), "old-job", time.Hour, state, func(s State) { state = s })
	if !errors.Is(err, ErrSessionChanged) || state != StateUnknown {
		t.Fatalf("err=%v state=%s", err, state)
	}
	state, _, err = c.Inspect(t.Context(), "old-job")
	if !errors.Is(err, ErrSessionChanged) || state != StateUnknown {
		t.Fatalf("inspection err=%v state=%s", err, state)
	}
}
func TestCompletedResultWinsOverDisconnectedOrRestartedBridge(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "already stored", true: "stored during failed GET"}[late], func(t *testing.T) {
			c := clientFixture(t)
			c.ExpectedSession = "original"
			write := func() {
				b, _ := json.Marshal(Result{Type: "result", ID: "complete-job", OK: true, BridgeSession: "original", Protocol: 1})
				if err := os.WriteFile(filepath.Join(config.JobsDir(), "complete-job.json"), b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if !late {
				write()
			}
			calls := 0
			c.http.Transport = fixtureTransport(func(*http.Request) (*http.Response, error) { calls++; write(); return nil, refused() })
			r, err := c.WaitAccepted(t.Context(), "complete-job", time.Hour, StateRunning, nil)
			if err != nil || r == nil || !r.OK {
				t.Fatalf("result=%v err=%v", r, err)
			}
			if calls != map[bool]int{false: 0, true: 1}[late] {
				t.Fatal(calls)
			}
		})
	}
}
func TestOlderBridgeWithoutSessionStillSupported(t *testing.T) {
	c := clientFixture(t)
	c.ExpectedSession = "original"
	c.http.Transport = fixtureTransport(func(*http.Request) (*http.Response, error) {
		return fixtureReply(`{"type":"result","id":"old-job","ok":true,"protocol":1}`), nil
	})
	r, err := c.WaitAccepted(t.Context(), "old-job", time.Hour, StateRunning, nil)
	if err != nil || r == nil || !r.OK {
		t.Fatalf("result=%v err=%v", r, err)
	}
}
func TestNativeSilenceRemainsBusyRatherThanDisconnected(t *testing.T) {
	c := clientFixture(t)
	c.http.Transport = fixtureTransport(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
	synctest.Test(t, func(t *testing.T) {
		state := StateRunning
		_, err := c.WaitAccepted(t.Context(), "blocked-native", 45*time.Second, state, func(s State) { state = s })
		if !errors.Is(err, ErrStillRunning) || errors.Is(err, ErrDisconnected) || state != StateBusy {
			t.Fatalf("err=%v state=%s", err, state)
		}
	})
}
