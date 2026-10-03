package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureArchive(t *testing.T, entries []*tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, h := range entries {
		if e := tw.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if h.Size > 0 {
			if _, e := tw.Write(bytes.Repeat([]byte("x"), int(h.Size))); e != nil {
				t.Fatal(e)
			}
		}
	}
	if e := tw.Close(); e != nil {
		t.Fatal(e)
	}
	if e := gz.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestMacBundleArchiveValidation(t *testing.T) {
	executable := &tar.Header{Name: "cav-cli-1.0.2/Cav.app/Contents/MacOS/cav", Mode: 0755, Size: 10, Typeflag: tar.TypeReg}
	if root, e := macBundleRoot(fixtureArchive(t, []*tar.Header{executable})); e != nil || root != "cav-cli-1.0.2/Cav.app" {
		t.Fatalf("%s %v", root, e)
	}
	for _, bad := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "/escape", Typeflag: tar.TypeReg}, {Name: "cav-cli-1.0.2/Cav.app/link", Linkname: "/tmp/target", Typeflag: tar.TypeSymlink}, {Name: "cav-cli-1.0.2/Cav.app/Contents/MacOS/cav", Mode: 0644, Typeflag: tar.TypeReg}, {Name: "cav", Mode: 0755, Typeflag: tar.TypeReg}} {
		if _, e := macBundleRoot(fixtureArchive(t, []*tar.Header{bad})); e == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
func TestFullBundleUpdatePreservesTreeAndSupportsNextUpdate(t *testing.T) {
	tools := t.TempDir()
	for _, name := range []string{"codesign", "xcrun", "spctl"} {
		script := "#!/bin/sh\nexit 0\n"
		if name == "codesign" {
			script = "#!/bin/sh\ncase \"$1\" in -d) echo 'Authority=Developer ID Application: Fixture'; echo 'Timestamp=fixture'; echo 'flags=0x10000(runtime)'; echo 'TeamIdentifier=FIXTURE';; esac\n"
		}
		if e := os.WriteFile(filepath.Join(tools, name), []byte(script), 0755); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("PATH", tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	self := filepath.Join(dir, "cav")
	if e := os.WriteFile(self, []byte("old"), 0755); e != nil {
		t.Fatal(e)
	}
	prefix := "cav-cli-1.0.2/Cav.app/Contents/"
	entries := []*tar.Header{{Name: prefix + "MacOS/cav", Mode: 0755, Size: 10, Typeflag: tar.TypeReg}, {Name: prefix + "Info.plist", Mode: 0644, Size: 10, Typeflag: tar.TypeReg}, {Name: prefix + "Resources/NOTICE", Mode: 0644, Size: 10, Typeflag: tar.TypeReg}, {Name: prefix + "_CodeSignature/CodeResources", Mode: 0644, Size: 10, Typeflag: tar.TypeReg}}
	launcher, e := installMacBundle(fixtureArchive(t, entries), self)
	if e != nil {
		t.Fatal(e)
	}
	installed, e := filepath.EvalSymlinks(self)
	si, se := os.Stat(installed)
	li, le := os.Stat(launcher)
	if e != nil || se != nil || le != nil || !os.SameFile(si, li) {
		t.Fatalf("link: %s %v", installed, e)
	}
	nextDir, e := macInstallDir(launcher)
	if e != nil || nextDir != dir {
		t.Fatalf("next update: %s %v", nextDir, e)
	}
	app := filepath.Dir(filepath.Dir(filepath.Dir(launcher)))
	for _, f := range []string{"Contents/Info.plist", "Contents/Resources/NOTICE", "Contents/_CodeSignature/CodeResources"} {
		if _, e = os.Stat(filepath.Join(app, f)); e != nil {
			t.Fatal(e)
		}
	}
	if !strings.Contains(launcher, ".cav-bundles") {
		t.Fatal(launcher)
	}
}
