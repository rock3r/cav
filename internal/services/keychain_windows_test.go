package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
)

const credPersistLocalMachine = 2

// writeTestCredential saves a generic credential the way cmdkey does (UTF-16LE blob) and
// removes it when the test ends.
func writeTestCredential(t *testing.T, target, user string, blob []byte) {
	t.Helper()
	tp, _ := syscall.UTF16PtrFromString(target)
	up, _ := syscall.UTF16PtrFromString(user)
	c := credential{Type: credTypeGeneric, TargetName: tp, UserName: up, Persist: credPersistLocalMachine,
		CredentialBlobSize: uint32(len(blob))}
	if len(blob) > 0 {
		c.CredentialBlob = &blob[0]
	}
	if r, _, err := procCredWrite.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		t.Fatalf("CredWriteW: %v", err)
	}
	t.Cleanup(func() { procCredDelete.Call(uintptr(unsafe.Pointer(tp)), credTypeGeneric, 0) })
}

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

func TestKeychainWindowsCredentialManager(t *testing.T) {
	target := fmt.Sprintf("cav-test-%d", time.Now().UnixNano())
	src := "keychain:" + target + "/openai"
	var ke *KeyError

	if _, err := Resolve(context.Background(), "openai", src); !errors.As(err, &ke) ||
		!strings.Contains(ke.Msg, "no Credential Manager entry") || !strings.Contains(ke.Fix, "cmdkey /generic:"+target) {
		t.Fatalf("missing: %v", err)
	}

	writeTestCredential(t, target, "OpenAI", utf16le("sk-from-wincred\n"))
	if v, err := Resolve(context.Background(), "openai", src); err != nil || v != "sk-from-wincred" {
		t.Fatalf("UTF-16 blob: got %q, %v", v, err)
	}
	if _, err := Resolve(context.Background(), "openai", "keychain:"+target+"/someone-else"); !errors.As(err, &ke) ||
		!strings.Contains(ke.Msg, "no Credential Manager entry") {
		t.Fatalf("other account: %v", err)
	}

	writeTestCredential(t, target, "openai", []byte("sk-utf8"))
	if v, err := Resolve(context.Background(), "openai", src); err != nil || v != "sk-utf8" {
		t.Fatalf("UTF-8 blob: got %q, %v", v, err)
	}

	writeTestCredential(t, target, "openai", nil)
	if _, err := Resolve(context.Background(), "openai", src); !errors.As(err, &ke) || !strings.Contains(ke.Msg, "entry is empty") {
		t.Fatalf("empty: %v", err)
	}
}

func TestCredErrorMapping(t *testing.T) {
	for _, c := range []struct {
		in   error
		want error
	}{
		{errorNotFound, errKeychainNoEntry},
		{errorAccessDenied, errKeychainDenied},
		{errorNoSuchLogonSession, errKeychainDenied},
	} {
		if got := credError(c.in); !errors.Is(got, c.want) {
			t.Errorf("%v: got %v, want %v", c.in, got, c.want)
		}
	}
	if got := credError(syscall.Errno(87)); errors.Is(got, errKeychainNoEntry) || errors.Is(got, errKeychainDenied) {
		t.Errorf("invalid parameter mapped to %v", got)
	}
}
