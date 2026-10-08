package bridge

import (
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/rock3r/cav/internal/cavapp"
)

// fakeCavalry makes the Cavalry process check report s, with no crash reports.
func fakeCavalry(t *testing.T, s cavapp.Status) {
	t.Helper()
	oldProbe, oldDir := cavapp.Probe, cavapp.CrashDir
	t.Cleanup(func() { cavapp.Probe, cavapp.CrashDir = oldProbe, oldDir })
	cavapp.Probe = func() cavapp.Status { return s }
	cavapp.CrashDir = t.TempDir()
}

// A refused connection says whether Cavalry itself runs, so nobody restarts only the
// bridge after Cavalry crashed.
func TestUnreachableSaysWhetherCavalryRuns(t *testing.T) {
	refusedErr := &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}
	for _, tc := range []struct {
		host string
		s    cavapp.Status
		want string
	}{
		{"127.0.0.1", cavapp.NotRunning, "Cavalry is not running (it may have crashed). Start Cavalry, then Scripts menu > cav-bridge"},
		{"localhost", cavapp.Running, "Cavalry is running but cav-bridge is not. In Cavalry, open Scripts menu > cav-bridge"},
		{"127.0.0.1", cavapp.Unknown, "Open Cavalry, then Scripts menu > cav-bridge"},
		// cav cannot see the processes of another machine.
		{"192.168.1.20", cavapp.NotRunning, "Open Cavalry, then Scripts menu > cav-bridge"},
	} {
		fakeCavalry(t, tc.s)
		err := unreachable(tc.host, "http://"+tc.host+":8723", refusedErr)
		if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s, %v: %v", tc.host, tc.s, err)
		}
	}
}

func TestProtocolError(t *testing.T) {
	for _, tc := range []struct {
		name string
		r    Result
		bad  bool
	}{
		{"bridge older than 1.0 sends no protocol", Result{OK: true}, false},
		{"same protocol", Result{OK: true, Protocol: Protocol}, false},
		{"bridge refused the request", Result{Error: &ScriptError{Code: "protocol", Message: "cav-bridge 2.0.0 speaks protocol 2, but this cav speaks protocol 1"}, Protocol: 2}, true},
		{"bridge reports another protocol", Result{OK: true, Protocol: Protocol + 1, BridgeVersion: "9.0.0"}, true},
		{"a script error is not a protocol error", Result{Error: &ScriptError{Message: "boom"}, Protocol: Protocol}, false},
	} {
		err := ProtocolError(&tc.r)
		if got := errors.Is(err, ErrProtocol); got != tc.bad {
			t.Errorf("%s: ProtocolError = %v", tc.name, err)
		}
	}
}

// A sandbox refuses the connection with EPERM; that is not a stopped bridge.
func TestUnreachableSandbox(t *testing.T) {
	fakeCavalry(t, cavapp.Running)
	blocked := unreachable("127.0.0.1", "http://127.0.0.1:8723", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.EPERM)})
	if !errors.Is(blocked, ErrBlocked) || !errors.Is(blocked, ErrUnavailable) {
		t.Errorf("EPERM: %v", blocked)
	}
	refused := unreachable("127.0.0.1", "http://127.0.0.1:8723", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)})
	if errors.Is(refused, ErrBlocked) || !errors.Is(refused, ErrUnavailable) {
		t.Errorf("ECONNREFUSED: %v", refused)
	}
}
