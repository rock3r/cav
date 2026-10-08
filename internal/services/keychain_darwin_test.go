package services

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type exitErr int

func (e exitErr) Error() string { return "exit status" }
func (e exitErr) ExitCode() int { return int(e) }

func TestKeychainDarwinClassifiesSecurityExitCodes(t *testing.T) {
	old := runCmd
	defer func() { runCmd = old }()
	for _, c := range []struct {
		code int
		want string
	}{
		{44, "no keychain entry"},
		{128, "keychain denied access"},
		{36, "keychain denied access"},
		{51, "keychain denied access"},
	} {
		runCmd = func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
			return nil, []byte("security: failed"), exitErr(c.code)
		}
		_, err := Resolve(context.Background(), "openai", "keychain:cav/openai")
		var ke *KeyError
		if !errors.As(err, &ke) || !strings.Contains(ke.Msg, c.want) {
			t.Errorf("exit %d: got %v, want %q", c.code, err, c.want)
		}
	}
	runCmd = func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name != "security" || strings.Join(args, " ") != "find-generic-password -s cav -a openai -w" {
			t.Fatalf("unexpected command %s %v", name, args)
		}
		return []byte("sk-from-keychain\n"), nil, nil
	}
	if v, err := Resolve(context.Background(), "openai", "keychain:cav/openai"); err != nil || v != "sk-from-keychain" {
		t.Fatalf("got %q, %v", v, err)
	}
}
