package services

import (
	"bytes"
	"strings"
	"unicode/utf16"
)

// decodeCredentialBlob reads a Windows credential blob. cmdkey and the Credential Manager
// window save UTF-16LE text; some other tools save UTF-8. An API key is ASCII, so a zero
// byte in an even-length blob means UTF-16LE.
func decodeCredentialBlob(b []byte) string {
	if len(b)%2 == 0 && bytes.IndexByte(b, 0) >= 0 {
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
		}
		return strings.TrimRight(string(utf16.Decode(u)), "\x00")
	}
	return strings.TrimRight(string(b), "\x00")
}
