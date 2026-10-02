package bridge

import (
	"errors"
	"testing"
)

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
