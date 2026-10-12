package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fakeReleases(t *testing.T, tag string, status int) *int {
	t.Helper()
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/repos/rock3r/cav/releases/latest" {
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

func TestDoctorReportsANewerReleaseOncePerDay(t *testing.T) {
	old := version
	version = "1.5.0"
	defer func() { version = old }()
	hits := fakeReleases(t, "v1.6.0", 200)
	now := time.Now()
	c := updateCheck(now)
	if c.OK || !c.Optional || c.Fix != "cav update" || !strings.Contains(c.Detail, "cav 1.6.0 is available (this is 1.5.0)") {
		t.Fatalf("newer release: %+v", c)
	}
	// The answer is cached for a day.
	if c2 := updateCheck(now.Add(23 * time.Hour)); c2 != c || *hits != 1 {
		t.Fatalf("cached check: %+v after %d requests", c2, *hits)
	}
	updateCheck(now.Add(25 * time.Hour))
	if *hits != 2 {
		t.Fatalf("a day later: %d requests, want 2", *hits)
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
