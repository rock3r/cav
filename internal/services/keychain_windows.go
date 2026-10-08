package services

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const (
	keychainName       = "Credential Manager"
	keychainDeniedHint = "run cav as the Windows user who saved the credential, in a logged-on session, then run again"
)

func keychainAddHint(svc, acct string) string {
	return fmt.Sprintf("cmdkey /generic:%s /user:%s /pass", quoteArg(svc), quoteArg(acct))
}

func quoteArg(s string) string {
	for _, r := range s {
		if r == ' ' || r == '\t' {
			return `"` + s + `"`
		}
	}
	return s
}

const (
	credTypeGeneric = 1

	errorAccessDenied       syscall.Errno = 5
	errorNotFound           syscall.Errno = 1168
	errorNoSuchLogonSession syscall.Errno = 1312
)

var (
	advapi32     = syscall.NewLazyDLL("advapi32.dll")
	procCredRead = advapi32.NewProc("CredReadW")
	procCredFree = advapi32.NewProc("CredFree")
)

// credential mirrors CREDENTIALW.
type credential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        syscall.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

// readKeychain reads the generic credential whose target is svc and whose user name is
// acct, as `cmdkey /generic:svc /user:acct /pass` saves it.
func readKeychain(_ context.Context, svc, acct string) (string, error) {
	target, err := syscall.UTF16PtrFromString(svc)
	if err != nil {
		return "", err
	}
	var c *credential
	r, _, callErr := procCredRead.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&c)))
	runtime.KeepAlive(target)
	if r == 0 {
		return "", credError(callErr)
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(c)))
	if user := utf16PtrToString(c.UserName); !strings.EqualFold(user, acct) {
		return "", errKeychainNoEntry
	}
	if c.CredentialBlobSize == 0 || c.CredentialBlob == nil {
		return "", nil
	}
	blob := unsafe.Slice(c.CredentialBlob, c.CredentialBlobSize)
	return decodeCredentialBlob(blob), nil
}

// credError maps a CredReadW failure to the shared keychain errors.
func credError(err error) error {
	switch err {
	case errorNotFound:
		return errKeychainNoEntry
	case errorAccessDenied, errorNoSuchLogonSession:
		return errKeychainDenied
	}
	return err
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var n int
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; n++ {
		ptr = unsafe.Add(ptr, 2)
	}
	return string(utf16.Decode(unsafe.Slice(p, n)))
}
