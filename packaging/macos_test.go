package packaging

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Mocked Apple tools check orchestration and layout only, never real notarization.
func TestMacPackagingAndInstall(t *testing.T) {
	r := root(t)
	tools := t.TempDir()
	log := filepath.Join(t.TempDir(), "tools.log")
	scripts := map[string]string{
		"uname":    "case \"$1\" in -s) echo Darwin;; -m) echo arm64;; esac",
		"codesign": "echo \"codesign $*\" >> \"$FIXTURE_TOOL_LOG\"; if [ \"$1\" = -d ]; then echo 'Authority=Developer ID Application: Fixture'; echo 'TeamIdentifier=FIXTURE'; echo 'Timestamp=fixture'; echo 'flags=0x10000(runtime)'; fi",
		"xcrun":    "echo \"xcrun $*\" >> \"$FIXTURE_TOOL_LOG\"; if [ \"$1 $2\" = 'notarytool submit' ]; then echo '{\"status\":\"Accepted\",\"id\":\"fixture\"}'; fi; if [ \"$1 $2\" = 'stapler staple' ]; then mkdir -p \"$3/Contents/_CodeSignature\"; echo ticket > \"$3/Contents/_CodeSignature/ticket\"; fi",
		"spctl":    "echo \"spctl $*\" >> \"$FIXTURE_TOOL_LOG\"",
		"ditto":    "if [ \"$1\" = -c ]; then for last do :; done; touch \"$last\"; else cp -Rp \"$1\" \"$2\"; fi",
	}
	for name, body := range scripts {
		if e := os.WriteFile(filepath.Join(tools, name), []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0755); e != nil {
			t.Fatal(e)
		}
	}
	fixture := t.TempDir()
	bin := filepath.Join(fixture, "cav")
	if e := os.WriteFile(bin, []byte("#!/bin/sh\necho setup-fixture\n"), 0755); e != nil {
		t.Fatal(e)
	}
	run := func(args []string, extra ...string) string {
		t.Helper()
		c := exec.Command(args[0], args[1:]...)
		c.Dir = r
		// Do not inherit Apple credentials from the operator environment.
		c.Env = []string{"PATH=" + tools + string(os.PathListSeparator) + os.Getenv("PATH"), "FIXTURE_TOOL_LOG=" + log, "TMPDIR=" + fixture}
		c.Env = append(c.Env, extra...)
		out, e := c.CombinedOutput()
		if e != nil {
			t.Fatalf("%s: %v\n%s", args, e, out)
		}
		return string(out)
	}
	archive := filepath.Join(fixture, "cav_darwin_arm64.tar.gz")
	run([]string{"bash", "tools/package-macos.sh", bin, "1.0.2", archive, "dev"})
	if _, e := os.Stat(log); !os.IsNotExist(e) {
		t.Fatal("dev packaging accessed Apple tools")
	}
	run([]string{"bash", "tools/package-macos.sh", bin, "1.0.2", archive, "release"}, "APPLE_DEVELOPER_IDENTITY=Developer ID Application: Fixture", "APPLE_NOTARY_API_KEY_PATH=/fixture/notary.p8", "APPLE_NOTARY_API_KEY_ID=fixture", "APPLE_NOTARY_API_ISSUER=fixture")
	lines, e := os.ReadFile(log)
	if e != nil {
		t.Fatal(e)
	}
	calls := string(lines)
	for _, required := range []string{"--options runtime --timestamp", "notarytool submit", "stapler staple", "stapler validate", "spctl --assess"} {
		if !strings.Contains(calls, required) {
			t.Fatalf("missing %s: %s", required, calls)
		}
	}
	if strings.Contains(calls, "--entitlements") || strings.Contains(calls, "--sign - ") {
		t.Fatal("release weakened its signing")
	}
	f, e := os.Open(archive)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	gz, e := gzip.NewReader(f)
	if e != nil {
		t.Fatal(e)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	members := map[string]*tar.Header{}
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		members[h.Name] = h
	}
	for _, name := range []string{"Contents/Info.plist", "Contents/Resources/NOTICE", "Contents/_CodeSignature/ticket", "Contents/MacOS/cav"} {
		if _, ok := members["cav-cli-1.0.2/Cav.app/"+name]; !ok {
			t.Fatal("missing bundle member " + name)
		}
	}
	if members["cav-cli-1.0.2/Cav.app/Contents/MacOS/cav"].Mode&0111 == 0 {
		t.Fatal("launcher lost execute permission")
	}
	// Use a local fixture mirror with a checksum, then run the actual installer.
	sum, e := exec.Command("shasum", "-a", "256", archive).Output()
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(fixture, "checksums.txt"), []byte(strings.Fields(string(sum))[0]+"  cav_darwin_arm64.tar.gz\n"), 0600); e != nil {
		t.Fatal(e)
	}
	installDir := filepath.Join(fixture, "bin")
	run([]string{"sh", "install.sh"}, "CAV_BASE_URL=file://"+fixture, "CAV_BIN_DIR="+installDir, "CAV_VERSION=v1.0.2")
	link, e := os.Readlink(filepath.Join(installDir, "cav"))
	if e != nil {
		t.Fatal(e)
	}
	app := filepath.Dir(filepath.Dir(filepath.Dir(link)))
	if _, e = os.Stat(filepath.Join(app, "Contents/_CodeSignature/ticket")); e != nil {
		t.Fatal("install stripped bundle ticket: " + e.Error())
	}
	run([]string{"sh", "install.sh"}, "CAV_BASE_URL=file://"+fixture, "CAV_BIN_DIR="+installDir, "CAV_VERSION=v1.0.2")
	newLink, e := os.Readlink(filepath.Join(installDir, "cav"))
	if e != nil || newLink == link {
		t.Fatal("second install did not switch a full bundle")
	}
	if _, e = os.Stat(link); e != nil {
		t.Fatal("old bundle was not retained")
	}
}
func TestReleaseRequiresCredentialsBeforeMutation(t *testing.T) {
	c := exec.Command("sh", "tools/release.sh", "v1.0.2")
	c.Dir = root(t)
	c.Env = []string{"PATH=" + os.Getenv("PATH")}
	out, e := c.CombinedOutput()
	if e == nil {
		t.Fatal("unsigned release accepted")
	}
	if !strings.Contains(string(out), "release signing required") && !strings.Contains(string(out), "signing host") {
		t.Fatalf("unexpected preflight failure %s", out)
	}
}
