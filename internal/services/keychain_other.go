//go:build !darwin && !windows

package services

import "context"

const (
	keychainName       = "keychain"
	keychainDeniedHint = ""
)

func keychainAddHint(svc, acct string) string { return "" }

func readKeychain(context.Context, string, string) (string, error) {
	return "", &KeyError{Msg: "keychain sources work on macOS and Windows only", Fix: "use env:NAME or op://… on this system"}
}
