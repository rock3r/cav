package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rock3r/cav/internal/services"
)

// makeBankReady fakes the pinned earshot checkout in the test's CAV_HOME, so setup is
// skipped.
func makeBankReady(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(bankDir(), "earshot", "forge", "bank"), 0o755); err != nil {
		t.Fatal(err)
	}
}

const bankListJSON = `{"version": "forge_v2r6+fit1+pluck1",
 "categories": {"pro_impact": {"target": "a deep cinematic boom impact", "use": "a cinematic hit: reveal, title slam"},
                "fit": {"target": "", "use": "recipes fitted from CC0 recordings"}},
 "sounds": [
  {"id": "pro_impact/deep", "category": "pro_impact", "member": "deep", "type": "forge", "dur_s": 0.67,
   "kinds": ["hit", "land"], "notes": "one clean gated hit"},
  {"id": "fit/bell/small_bell", "category": "fit", "member": "bell/small_bell", "type": "fit", "dur_s": 1.2,
   "kinds": ["bell"], "notes": "fitted from Small Bell #2 (CC0 1.0)",
   "source": {"title": "Small Bell #2", "author": "steffcaffrey", "license": "CC0 1.0",
              "url": "https://freesound.org/people/steffcaffrey/sounds/452379/"}},
  {"id": "whoosh_fast/crisp", "category": "whoosh_fast", "member": "crisp", "type": "forge", "dur_s": 0.5,
   "kinds": ["whoosh"], "notes": "a fast crisp air whoosh"}]}`

func fakeUVBankList(t *testing.T) {
	t.Helper()
	fakeUVOnPath(t, "#!/bin/sh\nprintf '%s' '"+bankListJSON+"'\n")
}

// The bank matches its ids, kinds and description texts, and a fit sound carries its CC0
// source licence and author.
func TestBankSearchMatchesDescriptions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	makeBankReady(t)
	fakeUVBankList(t)

	data := searchJSON(t, "--service", "bank", "deep", "boom")
	results := data["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("want only the impact, got %v", results)
	}
	r := results[0].(map[string]any)
	if r["id"] != "pro_impact/deep" || r["service"] != "bank" || r["licence"] != "MIT" || r["seconds"] != 0.67 {
		t.Fatalf("unexpected result: %v", r)
	}
	if data["service"] != "the sound bank" {
		t.Fatalf("service: %v", data["service"])
	}

	data = searchJSON(t, "--service", "bank", "bell")
	r = data["results"].([]any)[0].(map[string]any)
	if r["licence"] != "CC0-1.0" || r["author"] != "steffcaffrey" {
		t.Fatalf("the fit source must carry through: %v", r)
	}
}

// stubOpenverseEmpty answers the openverse audio search with no hits, so a plain search
// in a test does not hit the network.
func stubOpenverseEmpty(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/audio/") {
			w.Write([]byte(`{"results":[]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: toServer{u}}
	t.Cleanup(func() { services.HTTPClient = old })
}

// Once set up, the bank joins a plain search without --service.
func TestBankJoinsPlainSearchOnceReady(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	makeBankReady(t)
	fakeUVBankList(t)
	stubOpenverseEmpty(t)

	data := searchJSON(t, "crisp", "whoosh")
	results := data["results"].([]any)
	if len(results) != 1 || results[0].(map[string]any)["service"] != "bank" {
		t.Fatalf("want the bank hit in a plain search, got %v", results)
	}
}

// Before the first explicit --service bank, a plain search must not pay the setup.
func TestBankStaysOutOfPlainSearchUntilReady(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if bankReady() {
		t.Fatal("the bank must not be ready in a fresh CAV_HOME")
	}
	data := searchJSON(t, "--service", "local", "whoosh")
	rs, _ := data["results"].([]any) // JSON null when there are none
	if len(rs) != 0 {
		t.Fatalf("want no results, got %v", rs)
	}
}

// --max-seconds filters bank sounds like it filters the remote services.
func TestBankSearchHonoursMaxSeconds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	makeBankReady(t)
	fakeUVBankList(t)

	data := searchJSON(t, "--service", "bank", "--max-seconds", "1", "a")
	var ids []string
	for _, r := range data["results"].([]any) {
		ids = append(ids, r.(map[string]any)["id"].(string))
	}
	if len(ids) != 2 || ids[0] != "pro_impact/deep" || ids[1] != "whoosh_fast/crisp" {
		t.Fatalf("want the two sounds under a second, got %v", ids)
	}
}

// bankGet renders through the helper, never replaces an existing take, records the fit
// source, and surfaces the audit's warnings.
func TestBankGetRendersAndRecords(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	makeBankReady(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "fit-bell-small_bell.wav"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	fakeUVOnPath(t, `#!/bin/sh
out=""
for a in "$@"; do
  case "$a" in
    *.wav) out="$a" ;;
  esac
done
printf 'RIFFfake' > "$out"
printf '{"seconds": 1.2, "checks": {"ok": false, "warnings": ["tail: a second event 6.2 dB up"], "notes": []}, "source": {"title": "Small Bell #2", "author": "steffcaffrey", "license": "CC0 1.0", "url": "https://freesound.org/people/steffcaffrey/sounds/452379/"}}'
`)

	e, notes, err := bankGet(context.Background(), "fit/bell/small_bell", dir, 0, -1)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(e.Path) != "fit-bell-small_bell-2.wav" {
		t.Fatalf("a take never replaces an earlier one: %s", e.Path)
	}
	if e.Licence != "CC0-1.0" || e.Author != "steffcaffrey" || !e.CommercialOK || e.Title != "Small Bell #2" ||
		e.URL != "https://freesound.org/people/steffcaffrey/sounds/452379/" {
		t.Fatalf("fit source not recorded: %+v", e)
	}
	if e.Generated == nil || e.Generated.Provider != "earshot" || e.Generated.Prompt != "fit/bell/small_bell" {
		t.Fatalf("generated record: %+v", e.Generated)
	}
	if len(notes) != 1 || !strings.Contains(notes[0], "audit") {
		t.Fatalf("the audit warning must surface, got %v", notes)
	}
	if b, _ := os.ReadFile(e.Path); string(b) != "RIFFfake" {
		t.Fatalf("the renderer's file is not there: %q", b)
	}
}

// get rejects flag combinations that make no sense.
func TestBankGetFlagValidation(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	if err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"get", "--original", "bank:click/soft"}); err == nil ||
		!strings.Contains(err.Error(), "--original is for downloads") {
		t.Fatalf("want the --original error, got %v", err)
	}
	if err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"get", "--seconds", "2", "openverse:x"}); err == nil ||
		!strings.Contains(err.Error(), "--seconds and --seed are for bank sounds") {
		t.Fatalf("want the --seconds error, got %v", err)
	}
	if err := cmdSources(&app{}, "sfx", "audio", "sfx", []string{"get", "--seed", "3", "local:/x.wav"}); err == nil ||
		!strings.Contains(err.Error(), "--seconds and --seed are for bank sounds") {
		t.Fatalf("want the --seed error for a local ref, got %v", err)
	}
}

// --json reports the audit notes too, not only stderr.
func TestBankGetReportsAuditNotesInJSON(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake uv is a shell script")
	}
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	makeBankReady(t)
	fakeUVOnPath(t, `#!/bin/sh
out=""
for a in "$@"; do
  case "$a" in
    *.wav) out="$a" ;;
  esac
done
printf 'RIFFfake' > "$out"
printf '{"seconds": 0.5, "checks": {"ok": false, "warnings": ["ring: metallic comb ring in the tail"], "notes": []}, "source": null}'
`)

	a := &app{json: true}
	var err error
	out := captureStdout(t, func() {
		err = cmdSources(a, "sfx", "audio", "sfx", []string{"get", "bank:whoosh_fast/crisp"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if e := json.Unmarshal([]byte(out), &data); e != nil {
		t.Fatalf("not JSON: %v\n%s", e, out)
	}
	notes, _ := data["notes"].([]any)
	if len(notes) != 1 || !strings.Contains(notes[0].(string), "audit") {
		t.Fatalf("want the audit note in the JSON, got %v", data["notes"])
	}
}

// The bank holds sounds only: cav ref search --service bank is an error, not an empty
// success.
func TestRefSearchRejectsTheBank(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	err := cmdSources(&app{}, "ref", "image", "ref", []string{"search", "--service", "bank", "neon"})
	if err == nil || !strings.Contains(err.Error(), `unknown service "bank"`) {
		t.Fatalf("want the unknown-service error, got %v", err)
	}
}

// ensureBank downloads and extracts the pinned source once, atomically enough that the
// ready marker appears only after a full extraction.
func TestEnsureBankFetchesAndExtracts(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	write := func(name, body string) {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	write("earshot-x/earshot/forge/bank/forge_v2.json", "{}")
	write("earshot-x/earshot/forge/bank.py", "# bank")
	tw.Close()
	gz.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(buf.Bytes())
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: toServer{u}}
	t.Cleanup(func() { services.HTTPClient = old })

	dir, err := ensureBank(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bankReady() {
		t.Fatal("the ready marker must appear after extraction")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "earshot", "forge", "bank.py")); string(b) != "# bank" {
		t.Fatalf("extraction wrote %q", b)
	}
}

func TestEnsureBankReportsABadDownload(t *testing.T) {
	t.Setenv("CAV_HOME", t.TempDir())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	old := services.HTTPClient
	services.HTTPClient = &http.Client{Transport: toServer{u}}
	t.Cleanup(func() { services.HTTPClient = old })

	if _, err := ensureBank(context.Background()); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("want the status in the error, got %v", err)
	}
	if bankReady() {
		t.Fatal("a failed download must not mark the bank ready")
	}
}
