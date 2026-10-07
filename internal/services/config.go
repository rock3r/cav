// Package services chooses which outside service or local tool does each production job
// (image generation, music, sound search, listening) and finds the API key for it.
//
// The config file holds where a key lives, never the key itself. A key comes from an
// environment variable, the OS keychain or a 1Password reference, and is read only when a
// command needs it.
package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/rock3r/cav/internal/config"
)

// Config is ~/.cav/services.json.
type Config struct {
	// Order lists, per job, the services to try first to last. A job missing here uses
	// DefaultOrder.
	Order map[string][]string `json:"order,omitempty"`
	// Keys maps a service to where its key lives: env:NAME, keychain:service/account,
	// op://vault/item/field, or oauth. A service missing here uses its default variable.
	Keys map[string]string `json:"keys,omitempty"`
	// Endpoints holds settings for services that run on a server you choose, such as an
	// OpenAI-compatible server hosting Qwen3-Omni.
	Endpoints map[string]Endpoint `json:"endpoints,omitempty"`
}

type Endpoint struct {
	BaseURL string `json:"baseURL"`
	Model   string `json:"model,omitempty"`
}

// Jobs, in the order `cav config` prints them.
var Jobs = []string{"image", "image.alpha", "image.vector", "music", "sfx", "ref", "ears"}

// JobHelp says what each job is for.
var JobHelp = map[string]string{
	"image":        "storyboard frames and generated stills",
	"image.alpha":  "assets with a transparent background",
	"image.vector": "SVG assets",
	"music":        "music tracks",
	"sfx":          "sound effect and music search",
	"ref":          "reference image search for mood boards",
	"ears":         "written critique of a track",
}

// DefaultOrder is the order cav uses when the config does not set one: services with a
// key first, then free and local ones. A service without a key is skipped, so with no keys
// at all the free entries win.
var DefaultOrder = map[string][]string{
	"image":        {"gemini", "openai", "openrouter", "greybox"},
	"image.alpha":  {"openai", "recraft"},
	"image.vector": {"recraft", "vtracer", "potrace"},
	"music":        {"elevenlabs", "stability"},
	"sfx":          {"freesound", "openverse"},
	"ref":          {"pexels", "unsplash", "openverse", "wikimedia"},
	"ears":         {"gemini", "qwen-omni"},
}

func Path() string { return filepath.Join(config.Home(), "services.json") }

// Load reads the config. A missing file is an empty config, not an error.
func Load() (*Config, error) {
	c := &Config{}
	b, err := os.ReadFile(Path())
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("%s: %w", Path(), err)
	}
	return c, nil
}

// Save writes the config with owner-only permissions.
func (c *Config) Save() error {
	if err := os.MkdirAll(config.Home(), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

// OrderFor returns the effective order for a job.
func (c *Config) OrderFor(job string) []string {
	if o, ok := c.Order[job]; ok && len(o) > 0 {
		return o
	}
	return DefaultOrder[job]
}

// KeySource returns where the key of a service lives: the configured source, or the
// service's default environment variable. Empty means the service needs no key.
func (c *Config) KeySource(service string) string {
	if s, ok := c.Keys[service]; ok && s != "" {
		return s
	}
	if d, ok := Catalog[service]; ok && len(d.EnvVars) > 0 {
		for _, v := range d.EnvVars {
			if os.Getenv(v) != "" {
				return "env:" + v
			}
		}
		return "env:" + d.EnvVars[0]
	}
	return ""
}

// SetOrder validates and stores the order for a job.
func (c *Config) SetOrder(job string, names []string) error {
	if !slices.Contains(Jobs, job) {
		return fmt.Errorf("unknown job %q (jobs: %s)", job, strings.Join(Jobs, ", "))
	}
	if len(names) == 0 {
		return fmt.Errorf("give at least one service")
	}
	for _, n := range names {
		d, ok := Catalog[n]
		if !ok {
			return fmt.Errorf("unknown service %q (services: %s)", n, strings.Join(ServiceNames(), ", "))
		}
		if !slices.Contains(d.Jobs, job) {
			return fmt.Errorf("service %q cannot do job %q (it does: %s)", n, job, strings.Join(d.Jobs, ", "))
		}
	}
	if c.Order == nil {
		c.Order = map[string][]string{}
	}
	c.Order[job] = names
	return nil
}

// SetKey validates and stores a key source for a service.
func (c *Config) SetKey(service, source string) error {
	d, ok := Catalog[service]
	if !ok {
		return fmt.Errorf("unknown service %q (services: %s)", service, strings.Join(ServiceNames(), ", "))
	}
	if d.Kind != KindKey && d.Kind != KindEndpoint {
		return fmt.Errorf("service %q needs no key", service)
	}
	if _, err := ParseSource(source); err != nil {
		return err
	}
	if c.Keys == nil {
		c.Keys = map[string]string{}
	}
	c.Keys[service] = source
	return nil
}

// ServiceNames lists the catalog, sorted.
func ServiceNames() []string {
	var n []string
	for k := range Catalog {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}
