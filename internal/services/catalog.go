package services

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Kind int

const (
	KindKey      Kind = iota // a web API that needs a key
	KindFree                 // a web API that needs no key
	KindLocal                // a program on this machine
	KindBuiltin              // done by cav itself
	KindEndpoint             // a server you choose (base URL in the config), key optional
)

// Def describes one service.
type Def struct {
	Name    string
	Title   string
	Kind    Kind
	Jobs    []string
	EnvVars []string // default key variables, first match wins
	Binary  string   // for KindLocal
	Signup  string   // where to get a key
	// Check makes one free call that proves the key works. key is "" for free services.
	Check func(ctx context.Context, c *Config, key string) (string, error)
}

var Catalog = map[string]*Def{
	"gemini": {Title: "Google Gemini (Nano Banana image models, audio critique)", Kind: KindKey,
		Jobs: []string{"image", "ears"}, EnvVars: []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"},
		Signup: "https://aistudio.google.com/apikey",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Models []struct{ Name string } `json:"models"`
			}
			err := Do(ctx, "GET", "https://generativelanguage.googleapis.com/v1beta/models?pageSize=200", map[string]string{"x-goog-api-key": key}, nil, &r)
			if err != nil {
				return "", err
			}
			n := 0
			for _, m := range r.Models {
				if strings.Contains(m.Name, "image") {
					n++
				}
			}
			return fmt.Sprintf("%d models, %d image models", len(r.Models), n), nil
		}},
	"openai": {Title: "OpenAI (gpt-image: transparent PNG, text in images)", Kind: KindKey,
		Jobs: []string{"image", "image.alpha"}, EnvVars: []string{"OPENAI_API_KEY"},
		Signup: "https://platform.openai.com/api-keys",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Data []struct{ ID string } `json:"data"`
			}
			if err := Do(ctx, "GET", "https://api.openai.com/v1/models", bearer(key), nil, &r); err != nil {
				return "", err
			}
			var img []string
			for _, m := range r.Data {
				if strings.HasPrefix(m.ID, "gpt-image") {
					img = append(img, m.ID)
				}
			}
			return fmt.Sprintf("%d models; image: %s", len(r.Data), strings.Join(img, ", ")), nil
		}},
	"openrouter": {Title: "OpenRouter (one key for many image models)", Kind: KindKey,
		Jobs: []string{"image"}, EnvVars: []string{"OPENROUTER_API_KEY"},
		Signup: "https://openrouter.ai/keys",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Data struct {
					Label string   `json:"label"`
					Limit *float64 `json:"limit_remaining"`
				} `json:"data"`
			}
			if err := Do(ctx, "GET", "https://openrouter.ai/api/v1/key", bearer(key), nil, &r); err != nil {
				return "", err
			}
			if r.Data.Limit != nil {
				return fmt.Sprintf("key %q, %.2f credit left", r.Data.Label, *r.Data.Limit), nil
			}
			return fmt.Sprintf("key %q", r.Data.Label), nil
		}},
	"recraft": {Title: "Recraft (native SVG, background removal)", Kind: KindKey,
		Jobs: []string{"image.vector", "image.alpha", "image"}, EnvVars: []string{"RECRAFT_API_TOKEN", "RECRAFT_API_KEY"},
		Signup: "https://www.recraft.ai/profile/api",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Credits float64 `json:"credits"`
			}
			if err := Do(ctx, "GET", "https://external.api.recraft.ai/v1/users/me", bearer(key), nil, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("%.0f credits", r.Credits), nil
		}},
	"elevenlabs": {Title: "ElevenLabs (music with section plans, generated sound effects)", Kind: KindKey,
		Jobs: []string{"music", "sfx"}, EnvVars: []string{"ELEVENLABS_API_KEY", "XI_API_KEY"},
		Signup: "https://elevenlabs.io/app/settings/api-keys",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Subscription struct {
					Tier      string `json:"tier"`
					Used      int    `json:"character_count"`
					UsedLimit int    `json:"character_limit"`
				} `json:"subscription"`
			}
			if err := Do(ctx, "GET", "https://api.elevenlabs.io/v1/user", map[string]string{"xi-api-key": key}, nil, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("plan %s, %d of %d credits used", r.Subscription.Tier, r.Subscription.Used, r.Subscription.UsedLimit), nil
		}},
	"stability": {Title: "Stability AI (Stable Audio)", Kind: KindKey,
		Jobs: []string{"music"}, EnvVars: []string{"STABILITY_API_KEY"},
		Signup: "https://platform.stability.ai/account/keys",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Credits float64 `json:"credits"`
			}
			if err := Do(ctx, "GET", "https://api.stability.ai/v1/user/balance", bearer(key), nil, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("%.1f credits", r.Credits), nil
		}},
	"freesound": {Title: "Freesound (CC sound effects and music)", Kind: KindKey,
		Jobs: []string{"sfx"}, EnvVars: []string{"FREESOUND_API_KEY"},
		Signup: "https://freesound.org/apiv2/apply/",
		Check: func(ctx context.Context, c *Config, key string) (string, error) {
			var r struct {
				Count int `json:"count"`
			}
			u := "https://freesound.org/apiv2/search/text/?query=whoosh&page_size=1&fields=id"
			if err := Do(ctx, "GET", u, FreesoundAuth(c, key), nil, &r); err != nil {
				return "", err
			}
			detail := fmt.Sprintf("search works (%d results for \"whoosh\")", r.Count)
			if LoggedIn("freesound") {
				detail += "; logged in, original files available"
			} else {
				detail += "; previews only (cav config login freesound for originals)"
			}
			return detail, nil
		}},
	"openverse": {Title: "Openverse (CC images and audio, no key)", Kind: KindFree,
		Jobs: []string{"sfx", "ref"},
		Check: func(ctx context.Context, _ *Config, _ string) (string, error) {
			var r struct {
				Count int `json:"result_count"`
			}
			if err := Do(ctx, "GET", "https://api.openverse.org/v1/audio/?q=whoosh&page_size=1", nil, nil, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("reachable (%d audio results for \"whoosh\")", r.Count), nil
		}},
	"wikimedia": {Title: "Wikimedia Commons (no key)", Kind: KindFree,
		Jobs: []string{"ref"},
		Check: func(ctx context.Context, _ *Config, _ string) (string, error) {
			var r struct {
				Query struct {
					General struct {
						Sitename string `json:"sitename"`
					} `json:"general"`
				} `json:"query"`
			}
			if err := Do(ctx, "GET", "https://commons.wikimedia.org/w/api.php?action=query&meta=siteinfo&format=json", nil, nil, &r); err != nil {
				return "", err
			}
			return "reachable (" + r.Query.General.Sitename + ")", nil
		}},
	"pexels": {Title: "Pexels (stock photos and video)", Kind: KindKey,
		Jobs: []string{"ref"}, EnvVars: []string{"PEXELS_API_KEY"},
		Signup: "https://www.pexels.com/api/new/",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Total int `json:"total_results"`
			}
			if err := Do(ctx, "GET", "https://api.pexels.com/v1/search?query=neon&per_page=1", map[string]string{"Authorization": key}, nil, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("search works (%d results for \"neon\")", r.Total), nil
		}},
	"unsplash": {Title: "Unsplash (stock photos)", Kind: KindKey,
		Jobs: []string{"ref"}, EnvVars: []string{"UNSPLASH_ACCESS_KEY"},
		Signup: "https://unsplash.com/oauth/applications",
		Check: func(ctx context.Context, _ *Config, key string) (string, error) {
			var r struct {
				Total int `json:"total"`
			}
			if err := Do(ctx, "GET", "https://api.unsplash.com/search/photos?query=neon&per_page=1", map[string]string{"Authorization": "Client-ID " + key, "Accept-Version": "v1"}, nil, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("search works (%d results for \"neon\")", r.Total), nil
		}},
	"qwen-omni": {Title: "Qwen3-Omni Captioner on an OpenAI-compatible server you choose", Kind: KindEndpoint,
		Jobs: []string{"ears"}, EnvVars: []string{"QWEN_OMNI_API_KEY"},
		Check: func(ctx context.Context, c *Config, key string) (string, error) {
			ep, ok := c.Endpoints["qwen-omni"]
			if !ok || ep.BaseURL == "" {
				return "", &KeyError{Msg: "no server set", Missing: true, Fix: "cav config endpoint qwen-omni <base-url> [model]"}
			}
			var r struct {
				Data []struct{ ID string } `json:"data"`
			}
			h := map[string]string{}
			if key != "" {
				h = bearer(key)
			}
			if err := Do(ctx, "GET", strings.TrimRight(ep.BaseURL, "/")+"/models", h, nil, &r); err != nil {
				return "", err
			}
			return fmt.Sprintf("%d models at %s", len(r.Data), hostOf(ep.BaseURL)), nil
		}},
	"vtracer": {Title: "vtracer (trace PNG to SVG, local)", Kind: KindLocal, Jobs: []string{"image.vector"}, Binary: "vtracer",
		Signup: "cargo install vtracer, or https://github.com/visioncortex/vtracer/releases"},
	"potrace": {Title: "potrace (trace PNG to SVG in one colour, local)", Kind: KindLocal, Jobs: []string{"image.vector"}, Binary: "potrace",
		Signup: "brew install potrace"},
	"greybox": {Title: "Greybox frames drawn by cav (free)", Kind: KindBuiltin, Jobs: []string{"image"}},
}

func init() {
	for n, d := range Catalog {
		d.Name = n
	}
}

func bearer(key string) map[string]string { return map[string]string{"Authorization": "Bearer " + key} }

// FreesoundAuth sends an OAuth token as a bearer, and an API key as a token header.
func FreesoundAuth(c *Config, key string) map[string]string {
	if c != nil && c.Keys["freesound"] == "oauth" {
		return bearer(key)
	}
	return map[string]string{"Authorization": "Token " + key}
}

// Status is one service's health.
type Status struct {
	Service string `json:"service"`
	Job     string `json:"job,omitempty"`
	State   string `json:"state"` // ok, skip, warn, fail
	Source  string `json:"source,omitempty"`
	Detail  string `json:"detail"`
	Fix     string `json:"fix,omitempty"`
}

// Probe checks one service: its key (when it needs one) and one free call.
func Probe(ctx context.Context, c *Config, name string) Status {
	d := Catalog[name]
	st := Status{Service: name}
	switch d.Kind {
	case KindBuiltin:
		st.State, st.Detail = "ok", "built in"
		return st
	case KindLocal:
		p, err := lookPath(d.Binary)
		if err != nil {
			st.State, st.Detail, st.Fix = "skip", d.Binary+" not installed", d.Signup
			return st
		}
		st.State, st.Detail = "ok", p
		return st
	}
	key := ""
	if src := c.KeySource(name); src != "" {
		st.Source = describe(src)
		k, err := Resolve(ctx, name, src)
		var ke *KeyError
		if errors.As(err, &ke) && ke.Missing {
			if d.Kind == KindKey {
				st.State, st.Detail = "skip", ke.Msg
				st.Fix = ke.Fix
				if d.Signup != "" {
					st.Fix += " (get a key: " + d.Signup + ")"
				}
				return st
			}
		} else if err != nil {
			st.State, st.Detail = "fail", err.Error()
			if ke != nil {
				st.Fix = ke.Fix
			}
			return st
		}
		key = k
	}
	if d.Check == nil {
		st.State, st.Detail = "ok", "key found (no check available)"
		return st
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	detail, err := d.Check(cctx, c, key)
	if err != nil {
		var ke *KeyError
		if errors.As(err, &ke) && ke.Missing {
			st.State, st.Detail, st.Fix = "skip", ke.Msg, ke.Fix
			return st
		}
		st.State, st.Detail = "fail", redact(err.Error(), key)
		var he *HTTPError
		switch {
		case errors.As(err, &he) && he.KeyRefused() && d.Kind == KindKey:
			st.Detail = redact(fmt.Sprintf("the key was refused (HTTP %d): %s", he.Status, bodyMessage(he.Body)), key)
			st.Fix = "check the key, then cav config set-key " + name + " <source>"
		case strings.Contains(err.Error(), "x509") || strings.Contains(err.Error(), "certificate"):
			st.Fix = "a sandbox may block certificate checks: run cav outside the sandbox, or allow it to reach " + name
		}
		return st
	}
	st.State, st.Detail = "ok", detail
	return st
}

func describe(src string) string {
	s, err := ParseSource(src)
	if err != nil {
		return src
	}
	return s.Describe()
}

// redact removes a key that a server echoed back in an error.
func redact(s, key string) string {
	if len(key) >= 8 {
		s = strings.ReplaceAll(s, key, "[key]")
		s = strings.ReplaceAll(s, url.QueryEscape(key), "[key]")
	}
	return s
}

// Choice is the service picked for a job.
type Choice struct {
	Service string
	Key     string
	// Skipped lists the services tried before it, with the reason.
	Skipped []string
}

// Pick returns the first service in the job's order that is usable now: its key can be
// read, its program is installed, or it needs neither. It makes no network call; a
// service that fails during the job is reported by the caller.
func Pick(ctx context.Context, c *Config, job string, only string) (*Choice, error) {
	order := c.OrderFor(job)
	if only != "" {
		d, ok := Catalog[only]
		if !ok {
			return nil, fmt.Errorf("unknown service %q (services: %s)", only, strings.Join(ServiceNames(), ", "))
		}
		if !contains(d.Jobs, job) {
			return nil, fmt.Errorf("service %q cannot do %s (it does: %s)", only, job, strings.Join(d.Jobs, ", "))
		}
		order = []string{only}
	}
	ch := &Choice{}
	for _, name := range order {
		d, ok := Catalog[name]
		if !ok {
			ch.Skipped = append(ch.Skipped, name+": unknown service")
			continue
		}
		switch d.Kind {
		case KindBuiltin, KindFree:
			ch.Service = name
			return ch, nil
		case KindLocal:
			if _, err := lookPath(d.Binary); err != nil {
				ch.Skipped = append(ch.Skipped, name+": "+d.Binary+" not installed")
				continue
			}
			ch.Service = name
			return ch, nil
		case KindEndpoint:
			if ep, ok := c.Endpoints[name]; !ok || ep.BaseURL == "" {
				ch.Skipped = append(ch.Skipped, name+": no server set")
				continue
			}
		}
		key, err := Resolve(ctx, name, c.KeySource(name))
		if err != nil {
			var ke *KeyError
			if errors.As(err, &ke) && ke.Missing && d.Kind == KindEndpoint {
				ch.Service = name
				return ch, nil
			}
			ch.Skipped = append(ch.Skipped, name+": "+err.Error())
			continue
		}
		ch.Service, ch.Key = name, key
		return ch, nil
	}
	msg := "no service can do " + job + " right now"
	if len(ch.Skipped) > 0 {
		msg += ":\n  " + strings.Join(ch.Skipped, "\n  ")
	}
	return nil, errors.New(msg)
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
