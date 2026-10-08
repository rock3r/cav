package services

import (
	"errors"
	"strings"
	"testing"
)

func TestKeychainErrorIsTheSameOnEveryOS(t *testing.T) {
	var ke *KeyError
	if err := keychainError("cav", "openai", errKeychainNoEntry); !errors.As(err, &ke) ||
		!strings.Contains(ke.Msg, `no `+keychainName+` entry for service "cav" account "openai"`) || ke.Missing {
		t.Fatalf("no entry: %v", err)
	}
	if err := keychainError("cav", "openai", errKeychainDenied); !errors.As(err, &ke) ||
		!strings.Contains(ke.Msg, keychainName+` denied access to service "cav" account "openai"`) {
		t.Fatalf("denied: %v", err)
	}
	if err := keychainError("cav", "openai", errors.New("boom")); !errors.As(err, &ke) || !strings.Contains(ke.Msg, "read failed: boom") {
		t.Fatalf("other: %v", err)
	}
}

func TestDecodeCredentialBlob(t *testing.T) {
	utf16le := []byte{'s', 0, 'k', 0, '-', 0, '1', 0}
	for _, c := range []struct {
		in   []byte
		want string
	}{
		{utf16le, "sk-1"},
		{append(utf16le, 0, 0), "sk-1"},
		{[]byte("sk-1"), "sk-1"},
		{[]byte("sk-12"), "sk-12"},
		{nil, ""},
	} {
		if got := decodeCredentialBlob(c.in); got != c.want {
			t.Errorf("%v: got %q, want %q", c.in, got, c.want)
		}
	}
}
