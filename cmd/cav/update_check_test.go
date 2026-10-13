package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fakeReleases(t *testing.T, tag string, status int) *int {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/repos/"+repo()+"/releases/latest" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"tag_name": %q, "html_url": "https://example.test/%s"}`, tag, tag)
	}))
	t.Cleanup(srv.Close)
	old := releaseAPI
	releaseAPI = srv.URL
	t.Cleanup(func() { releaseAPI = old })
	t.Setenv("CAV_HOME", t.TempDir())
	t.Setenv("CAV_REPO", "")
	t.Setenv("CAV_NO_UPDATE_CHECK", "")
	return &hits
}

func TestDoctorReportsANewerReleaseOncePerHour(t *testing.T) {
	old := version
	version = "1.5.0"
	defer func() { version = old }()
	hits := fakeReleases(t, "v1.6.0", 200)
	now := time.Now()
	c := updateCheck(now)
	if c.OK || !c.Optional || c.Fix != "cav update" || !strings.Contains(c.Detail, "cav 1.6.0 is available (this is 1.5.0)") {
		t.Fatalf("newer release: %+v", c)
	}
	// The answer is cached for an hour.
	if c2 := updateCheck(now.Add(59 * time.Minute)); c2 != c || *hits != 1 {
		t.Fatalf("cached check: %+v after %d requests", c2, *hits)
	}
	updateCheck(now.Add(61 * time.Minute))
	if *hits != 2 {
		t.Fatalf("an hour later: %d requests, want 2", *hits)
	}
}

func TestDoctorIgnoresACacheOlderThanTheRunningRelease(t *testing.T) {
	old := version
	version = "1.6.0"
	defer func() { version = old }()
	hits := fakeReleases(t, "v1.6.0", 200)
	now := time.Now()
	// The cache was written a minute ago, before cav 1.6.0 was installed.
	cache := fmt.Sprintf(`{"checkedAt":%q,"repo":%q,"latest":"1.5.0","url":"x"}`,
		now.Add(-time.Minute).Format(time.RFC3339Nano), repo())
	if err := os.WriteFile(updateCachePath(), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	c := updateCheck(now)
	if *hits != 1 || !strings.Contains(c.Detail, "1.6.0 is the latest release") {
		t.Fatalf("stale cache: %+v after %d requests", c, *hits)
	}
}

func TestDoctorSaysUpToDate(t *testing.T) {
	old := version
	version = "1.5.0"
	defer func() { version = old }()
	fakeReleases(t, "v1.5.0", 200)
	if c := updateCheck(time.Now()); !c.OK || !strings.Contains(c.Detail, "1.5.0 is the latest release") {
		t.Fatalf("up to date: %+v", c)
	}
}

func TestDoctorUpdateCheckNeverFailsAndRetriesAfterAnHour(t *testing.T) {
	hits := fakeReleases(t, "", 500)
	now := time.Now()
	c := updateCheck(now)
	if !c.OK || !strings.Contains(c.Detail, "could not check") {
		t.Fatalf("failed check: %+v", c)
	}
	updateCheck(now.Add(30 * time.Minute))
	if *hits != 1 {
		t.Fatalf("retried within the hour: %d requests", *hits)
	}
	updateCheck(now.Add(61 * time.Minute))
	if *hits != 2 {
		t.Fatalf("no retry after an hour: %d requests", *hits)
	}
}

func TestDoctorUpdateCheckCanBeTurnedOff(t *testing.T) {
	hits := fakeReleases(t, "v9.0.0", 200)
	t.Setenv("CAV_NO_UPDATE_CHECK", "1")
	if c := updateCheck(time.Now()); !c.OK || *hits != 0 || !strings.Contains(c.Detail, "CAV_NO_UPDATE_CHECK") {
		t.Fatalf("turned off: %+v after %d requests", c, *hits)
	}
}

func TestDoctorUpdateCacheBelongsToOneRepository(t *testing.T) {
	hits := fakeReleases(t, "v1.5.0", 200)
	now := time.Now()
	updateCheck(now)
	t.Setenv("CAV_REPO", "someone/fork")
	updateCheck(now.Add(time.Minute))
	if *hits != 2 {
		t.Fatalf("another CAV_REPO reused the cache: %d requests", *hits)
	}
}

func TestDoctorIgnoresABrokenUpdateCache(t *testing.T) {
	hits := fakeReleases(t, "v1.5.0", 200)
	now := time.Now()
	for _, broken := range []string{
		`{"checkedAt":"` + now.Format(time.RFC3339Nano) + `","repo":"rock3r/cav","latest":`,
		`{"checkedAt":"` + now.Format(time.RFC3339Nano) + `","repo":"rock3r/cav"}`,
	} {
		if err := os.WriteFile(updateCachePath(), []byte(broken), 0o600); err != nil {
			t.Fatal(err)
		}
		updateCheck(now.Add(time.Minute))
	}
	if *hits != 2 {
		t.Fatalf("a broken cache was trusted: %d requests, want 2", *hits)
	}
}

func TestDoctorSaysWhenThisBuildIsNewerThanTheLatestRelease(t *testing.T) {
	old := version
	version = "1.5.1"
	defer func() { version = old }()
	fakeReleases(t, "v1.5.0", 200)
	if c := updateCheck(time.Now()); !c.OK || !strings.Contains(c.Detail, "cav 1.5.1 is newer than the latest release (1.5.0)") {
		t.Fatalf("newer build: %+v", c)
	}
}
