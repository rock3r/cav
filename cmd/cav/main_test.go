package main

import (
	"strings"
	"testing"
)

func TestHelperSignature(t *testing.T) {
	for _, name := range []string{"rect", "text", "create", "clear", "keys"} {
		if sig := helperSignature(name); !strings.HasPrefix(sig, "cav."+name+"(") {
			t.Errorf("helperSignature(%q) = %q", name, sig)
		}
	}
	if sig := helperSignature("noSuchHelper"); sig != "" {
		t.Errorf("unknown helper gave %q", sig)
	}
}
