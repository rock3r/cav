package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Source is a parsed key location.
type Source struct {
	Kind string // env, keychain, op, oauth
	Ref  string // variable name, service/account, op:// reference, or service name
}

// Describe names the source without its value, for reports.
func (s Source) Describe() string {
	switch s.Kind {
	case "env":
		return "env " + s.Ref
	case "keychain":
		return "keychain " + s.Ref
	case "op":
		return "1Password " + s.Ref
	case "oauth":
		return "OAuth login"
	}
	return s.Kind
}

// ParseSource reads env:NAME, keychain:service/account, op://vault/item/field or oauth.
func ParseSource(s string) (Source, error) {
	switch {
	case strings.HasPrefix(s, "env:") && len(s) > 4:
		return Source{"env", s[4:]}, nil
	case strings.HasPrefix(s, "keychain:"):
		ref := s[len("keychain:"):]
		if i := strings.Index(ref, "/"); i <= 0 || i == len(ref)-1 {
			return Source{}, fmt.Errorf("keychain source must be keychain:service/account, got %q", s)
		}
		return Source{"keychain", ref}, nil
	case strings.HasPrefix(s, "op://"):
		if strings.Count(s[len("op://"):], "/") < 2 {
			return Source{}, fmt.Errorf("1Password source must be op://vault/item/field, got %q", s)
		}
		return Source{"op", s}, nil
	case s == "oauth":
		return Source{"oauth", ""}, nil
	}
	return Source{}, fmt.Errorf("unknown key source %q: use env:NAME, keychain:service/account, op://vault/item/field or oauth", s)
}

// KeyError says why a key could not be read, and how to fix it.
type KeyError struct {
	Msg string
	Fix string
	// Missing is true when no key is configured at all (as opposed to a broken source).
	Missing bool
}

func (e *KeyError) Error() string { return e.Msg }

// runCmd runs a helper program; tests replace it.
var runCmd = func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error) {
	var o, e bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = &o, &e
	err = cmd.Run()
	return o.Bytes(), e.Bytes(), err
}

var lookPath = exec.LookPath

// Resolve reads the key from its source. The value never appears in errors.
func Resolve(ctx context.Context, service, source string) (string, error) {
	if source == "" {
		return "", nil
	}
	src, err := ParseSource(source)
	if err != nil {
		return "", &KeyError{Msg: err.Error(), Fix: "cav config set-key " + service + " <source>"}
	}
	switch src.Kind {
	case "env":
		v := strings.TrimSpace(os.Getenv(src.Ref))
		if v == "" {
			return "", &KeyError{Msg: "no key: " + src.Ref + " is not set", Missing: true,
				Fix: "export " + src.Ref + "=…, or cav config set-key " + service + " op://vault/item/field"}
		}
		return v, nil
	case "keychain":
		return keychainRead(ctx, service, src.Ref)
	case "op":
		return opRead(ctx, service, src.Ref)
	case "oauth":
		return oauthToken(ctx, service)
	}
	return "", &KeyError{Msg: "unsupported key source " + src.Kind}
}

func keychainRead(ctx context.Context, service, ref string) (string, error) {
	svc, acct, _ := strings.Cut(ref, "/")
	if runtime.GOOS != "darwin" {
		return "", &KeyError{Msg: "keychain sources work on macOS only", Fix: "use env:NAME or op://… on this system"}
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, _, err := runCmd(ctx, "security", "find-generic-password", "-s", svc, "-a", acct, "-w")
	if err != nil {
		return "", &KeyError{Msg: fmt.Sprintf("no keychain entry for service %q account %q", svc, acct),
			Fix: fmt.Sprintf("security add-generic-password -s %q -a %q -w", svc, acct)}
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", &KeyError{Msg: "the keychain entry is empty"}
	}
	return v, nil
}

func opRead(ctx context.Context, service, ref string) (string, error) {
	if _, err := lookPath("op"); err != nil {
		return "", &KeyError{Msg: "the 1Password CLI (op) is not installed", Fix: "install it: https://developer.1password.com/docs/cli/get-started/"}
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	out, stderr, err := runCmd(ctx, "op", "read", "--no-newline", ref)
	if err != nil {
		msg := strings.ToLower(string(stderr))
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return "", &KeyError{Msg: "1Password did not answer (it may be waiting for approval)", Fix: "unlock 1Password, approve the request, and run again"}
		case strings.Contains(msg, "not signed in"), strings.Contains(msg, "locked"), strings.Contains(msg, "authorization"), strings.Contains(msg, "sign in"):
			return "", &KeyError{Msg: "1Password is locked or not signed in", Fix: "unlock 1Password (or run `op signin`), then run again"}
		case strings.Contains(msg, "isn't an item"), strings.Contains(msg, "could not find"), strings.Contains(msg, "no item"), strings.Contains(msg, "does not have a field"):
			return "", &KeyError{Msg: "1Password has no item or field at " + ref, Fix: "check the reference with `op item get`, then cav config set-key " + service + " op://vault/item/field"}
		}
		return "", &KeyError{Msg: "op read failed: " + firstLine(string(stderr)), Fix: "run `op read '" + ref + "'` yourself to see the problem"}
	}
	v := strings.TrimSpace(string(out))
	if v == "" {
		return "", &KeyError{Msg: "the 1Password field " + ref + " is empty"}
	}
	return v, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
