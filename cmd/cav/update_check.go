package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/config"
)

// cav doctor asks GitHub for the latest release at most once a day, so an agent that runs
// doctor learns about an update. A failed check is remembered for an hour, so an offline
// machine or a firewall does not slow down every doctor run. CAV_NO_UPDATE_CHECK=1 turns
// the check off. The check never makes doctor fail.
const (
	updateCheckEvery      = 24 * time.Hour
	updateCheckRetryAfter = time.Hour
	updateCheckTimeout    = 3 * time.Second
)

type updateCache struct {
	CheckedAt time.Time `json:"checkedAt"`
	Repo      string    `json:"repo"`
	Latest    string    `json:"latest,omitempty"`
	URL       string    `json:"url,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func updateCachePath() string { return filepath.Join(config.Home(), "update-check.json") }

func updateCheck(now time.Time) check {
	c := check{Name: "update", OK: true, Optional: true}
	if os.Getenv("CAV_NO_UPDATE_CHECK") != "" {
		c.Detail = "not checked (CAV_NO_UPDATE_CHECK is set)"
		return c
	}
	var cache updateCache
	// A cache that does not decode, or that has neither an answer nor an error, does not count.
	if b, err := os.ReadFile(updateCachePath()); err == nil {
		if json.Unmarshal(b, &cache) != nil || cache.Latest == "" && cache.Error == "" {
			cache = updateCache{}
		}
	}
	fresh := cache.Error == "" && now.Sub(cache.CheckedAt) < updateCheckEvery ||
		cache.Error != "" && now.Sub(cache.CheckedAt) < updateCheckRetryAfter
	// A cache for another CAV_REPO, or from the future, does not count.
	if !fresh || cache.Repo != repo() || cache.CheckedAt.After(now) {
		cache = updateCache{CheckedAt: now, Repo: repo()}
		if r, err := fetchReleaseWithin("", updateCheckTimeout); err != nil {
			cache.Error = err.Error()
		} else {
			cache.Latest, cache.URL = trimV(r.Tag), r.URL
		}
		if b, err := json.Marshal(cache); err == nil {
			_ = os.MkdirAll(config.Home(), 0o700)
			_ = os.WriteFile(updateCachePath(), b, 0o600)
		}
	}
	switch {
	case cache.Error != "":
		c.Detail = "could not check for a newer cav: " + cache.Error
	case isNewerRelease(cache.Latest):
		c.OK = false
		c.Detail = "cav " + cache.Latest + " is available (this is " + version + "): " + cache.URL
		c.Fix = "cav update"
	case cache.Latest == strings.TrimSuffix(version, "-dev"):
		c.Detail = "cav " + version + " is the latest release"
	default:
		c.Detail = "cav " + version + " is newer than the latest release (" + cache.Latest + ")"
	}
	return c
}

func trimV(tag string) string {
	if len(tag) > 0 && tag[0] == 'v' {
		return tag[1:]
	}
	return tag
}
