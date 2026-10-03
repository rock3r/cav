package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// The GitHub repository that publishes releases. Override with CAV_REPO=owner/name.
const defaultRepo = "rock3r/cav"

func repo() string {
	if r := os.Getenv("CAV_REPO"); r != "" {
		return r
	}
	return defaultRepo
}

func init() {
	register(command{
		name:    "update",
		args:    "[--yes] [--version vX.Y.Z]",
		summary: "Download and install a newer cav release (asks first; never automatic).",
		run:     cmdUpdate,
	})
	longHelp["update"] = `
cav version --check   only reports whether a newer release exists
cav update            shows what would change and asks before installing
cav update --yes      installs without asking (for scripts)
The download is checked against the release's checksums.txt. After replacing the
installation, cav runs "cav setup" so the Cavalry bridge script is updated too; restart
the bridge window in Cavalry when setup says so. cav never updates itself silently.`
}

type release struct {
	Tag    string `json:"tag_name"`
	URL    string `json:"html_url"`
	Body   string `json:"body"`
	Assets []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchRelease(tag string) (*release, error) {
	url := "https://api.github.com/repos/" + repo() + "/releases/latest"
	if tag != "" {
		url = "https://api.github.com/repos/" + repo() + "/releases/tags/" + tag
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("no published release found for %s", repo())
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub answered HTTP %d", resp.StatusCode)
	}
	var r release
	return &r, json.NewDecoder(resp.Body).Decode(&r)
}

func checkUpdate(a *app, data map[string]any) error {
	r, err := fetchRelease("")
	if err != nil {
		data["updateCheck"] = err.Error()
		a.emit(data, func() {
			fmt.Printf("cav %s\nupdate check failed: %v\n", version, err)
		})
		return nil
	}
	latest := strings.TrimPrefix(r.Tag, "v")
	newer := versionLess(strings.TrimSuffix(version, "-dev"), latest) || (strings.HasSuffix(version, "-dev") && !versionLess(latest, strings.TrimSuffix(version, "-dev")) && latest != strings.TrimSuffix(version, "-dev"))
	data["latest"] = latest
	data["updateAvailable"] = newer
	data["releaseURL"] = r.URL
	a.emit(data, func() {
		fmt.Printf("cav %s, latest release %s\n", version, latest)
		if newer {
			fmt.Printf("an update is available: run `cav update` (%s)\n", r.URL)
		} else {
			fmt.Println("cav is up to date")
		}
	})
	return nil
}

func assetName() string {
	ext := ".tar.gz"
	if runtime.GOOS == "windows" {
		ext = ".zip"
	}
	return fmt.Sprintf("cav_%s_%s%s", runtime.GOOS, runtime.GOARCH, ext)
}

func cmdUpdate(a *app, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "install without asking")
	tag := fs.String("version", "", "install this release tag instead of the latest")
	if _, err := parseFlags(fs, args); err != nil {
		return err
	}
	r, err := fetchRelease(*tag)
	if err != nil {
		return fail(exitError, "update check failed: "+err.Error(), "check the internet connection, or set CAV_REPO")
	}
	latest := strings.TrimPrefix(r.Tag, "v")
	if *tag == "" && !versionLess(strings.TrimSuffix(version, "-dev"), latest) && !strings.HasSuffix(version, "-dev") {
		a.emit(map[string]any{"version": version, "latest": latest, "updated": false}, func() {
			fmt.Printf("cav %s is up to date\n", version)
		})
		return nil
	}
	var assetURL, sumsURL string
	for _, as := range r.Assets {
		switch as.Name {
		case assetName():
			assetURL = as.URL
		case "checksums.txt":
			sumsURL = as.URL
		}
	}
	if assetURL == "" || sumsURL == "" {
		return fail(exitError, "release "+r.Tag+" has no "+assetName()+" or checksums.txt", r.URL)
	}
	if !*yes {
		if a.json {
			return fail(exitError, "update needs confirmation", "run `cav update --yes` to install "+r.Tag)
		}
		fmt.Printf("cav %s -> %s\n%s\n\nInstall it? [y/N] ", version, latest, firstParagraph(r.Body))
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y") {
			fmt.Println("not updated")
			return nil
		}
	}
	archive, err := download(assetURL)
	if err != nil {
		return err
	}
	sums, err := download(sumsURL)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(archive)
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[1] == assetName() {
			want = f[0]
		}
	}
	if want == "" || want != hex.EncodeToString(sum[:]) {
		return fail(exitError, "checksum mismatch for "+assetName()+"; not installed", "try again later, or install manually from "+r.URL)
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	self, err = filepath.EvalSymlinks(self)
	if err != nil {
		return err
	}
	if runtime.GOOS == "darwin" {
		self, err = installMacBundle(archive, self)
		if err != nil {
			return fail(exitError, "bundle update failed: "+err.Error(), "use the current install.sh to install the complete signed bundle from "+r.URL)
		}
	} else {
		bin, e := extractBinary(archive)
		if e != nil {
			return e
		}
		if err = replaceBinary(self, bin); err != nil {
			return fail(exitError, "cannot replace "+self+": "+err.Error(), "install manually from "+r.URL)
		}
	}
	// Run setup with the new binary so the bridge script matches.
	out, _ := exec.Command(self, "setup", "--no-docs").CombinedOutput()
	a.emit(map[string]any{"updated": true, "from": version, "to": latest, "setup": string(out)}, func() {
		fmt.Printf("updated cav %s -> %s\n%s", version, latest, out)
	})
	return nil
}

func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "\n\n"); i > 0 {
		s = s[:i]
	}
	if len(s) > 600 {
		s = s[:600] + "…"
	}
	return s
}

func download(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 200<<20))
}

func extractBinary(archive []byte) ([]byte, error) {
	name := "cav"
	if runtime.GOOS == "windows" {
		name = "cav.exe"
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if filepath.Base(f.Name) == name {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, fmt.Errorf("%s not found in the archive", name)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("%s not found in the archive", name)
		}
		if filepath.Base(h.Name) == name {
			return io.ReadAll(tr)
		}
	}
}

// replaceBinary swaps the running executable. Windows cannot overwrite a running
// .exe, but it can rename it, so the old file is moved aside first.
func replaceBinary(self string, bin []byte) error {
	tmp := self + ".new"
	if err := os.WriteFile(tmp, bin, 0o755); err != nil {
		return err
	}
	old := self + ".old"
	_ = os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, self); err != nil {
		_ = os.Rename(old, self)
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(old)
	}
	return nil
}
