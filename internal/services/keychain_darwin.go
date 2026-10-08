package services

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	keychainName       = "keychain"
	keychainDeniedHint = "unlock the login keychain and allow the read when macOS asks, then run again"
)

func keychainAddHint(svc, acct string) string {
	return fmt.Sprintf("security add-generic-password -s %q -a %q -w", svc, acct)
}

// readKeychain asks `security` for the password. Its exit status is the low byte of the
// OSStatus: 44 is errSecItemNotFound; 128 (user canceled), 36 (interaction not allowed)
// and 51 (auth failed) mean the keychain refused.
func readKeychain(ctx context.Context, svc, acct string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, stderr, err := runCmd(ctx, "security", "find-generic-password", "-s", svc, "-a", acct, "-w")
	if err == nil {
		return string(out), nil
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "", &KeyError{Msg: "the keychain did not answer (it may be waiting for approval)",
			Fix: "allow the read when macOS asks, then run again"}
	}
	var ec interface{ ExitCode() int }
	if errors.As(err, &ec) {
		switch ec.ExitCode() {
		case 44:
			return "", errKeychainNoEntry
		case 128, 36, 51:
			return "", errKeychainDenied
		}
	}
	if msg := firstLine(string(stderr)); msg != "" {
		return "", errors.New(msg)
	}
	return "", err
}
