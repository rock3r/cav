package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rock3r/cav/internal/services"
)

func init() {
	register(command{
		name:    "config",
		args:    "[show | check [service...] | set-key <service> <source> | unset-key <service> | order <job> <s1,s2,...> | model <service> [id] | endpoint <service> <base-url> [model] | login <service>]",
		summary: "Choose the services that make images, music and sound, and where their keys live.",
		run:     cmdConfig,
	})
	longHelp["config"] = `
cav picks one service per job. Jobs: image, image.alpha (transparent PNG), image.vector
(SVG), music, sfx (sound search), ref (reference images), ears (critique of a track).
Each job has an order; cav uses the first service it can use now, so everything works with
no keys (greybox frames, Openverse, Wikimedia) and gets better as you add keys.

  cav config                        show the order per job and where each key lives
  cav config check [service...]     make one free call per service and report what works
  cav config set-key <service> <source>
        source is where the key lives, never the key itself:
          env:NAME                  an environment variable
          keychain:service/account  the macOS keychain
          op://vault/item/field     1Password (read with "op read" when a command needs it)
          oauth                     a login made with "cav config login <service>"
  cav config unset-key <service>    go back to the default variable
  cav config order <job> a,b,c      try a, then b, then c for that job
  cav config model gemini gemini-3-pro-image
                                    the model a service uses (no id: cav's default)
  cav config endpoint qwen-omni <base-url> [model]
                                    an OpenAI-compatible server you run or rent
  cav config login freesound --client-id ID [--secret SOURCE]
                                    OAuth login, needed to download Freesound originals

Without a set key, each service reads its usual variable (GEMINI_API_KEY, OPENAI_API_KEY,
ELEVENLABS_API_KEY, FREESOUND_API_KEY, PEXELS_API_KEY, ...). The config is
~/.cav/services.json; cav never prints, logs or writes a key value.
"cav doctor --services" adds the same checks to the doctor report.`
}

func cmdConfig(a *app, args []string) error {
	sub := "show"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	c, err := services.Load()
	if err != nil {
		return err
	}
	switch sub {
	case "show":
		return configShow(a, c)
	case "check":
		st := checkServices(c, args)
		a.emit(map[string]any{"services": st}, func() { printServiceStatus(st) })
		for _, s := range st {
			if s.State == "fail" {
				return &cliError{code: exitError, msg: "", data: map[string]any{"services": st}}
			}
		}
		return nil
	case "set-key":
		if len(args) != 2 {
			return usageErr("usage: cav config set-key <service> <source>")
		}
		if err := c.SetKey(args[0], args[1]); err != nil {
			return usageErr("%v", err)
		}
		return saveAndReport(a, c, fmt.Sprintf("%s key: %s", args[0], args[1]))
	case "unset-key":
		if len(args) != 1 {
			return usageErr("usage: cav config unset-key <service>")
		}
		delete(c.Keys, args[0])
		return saveAndReport(a, c, args[0]+" key: default ("+c.KeySource(args[0])+")")
	case "order":
		if len(args) != 2 {
			return usageErr("usage: cav config order <job> <service1,service2,...>")
		}
		if err := c.SetOrder(args[0], splitList(args[1])); err != nil {
			return usageErr("%v", err)
		}
		return saveAndReport(a, c, args[0]+": "+strings.Join(c.OrderFor(args[0]), " > "))
	case "endpoint":
		if len(args) < 2 || len(args) > 3 {
			return usageErr("usage: cav config endpoint <service> <base-url> [model]")
		}
		d, ok := services.Catalog[args[0]]
		if !ok || d.Kind != services.KindEndpoint {
			return usageErr("%s does not take a server address", args[0])
		}
		ep := services.Endpoint{BaseURL: args[1]}
		if len(args) == 3 {
			ep.Model = args[2]
		}
		if c.Endpoints == nil {
			c.Endpoints = map[string]services.Endpoint{}
		}
		c.Endpoints[args[0]] = ep
		return saveAndReport(a, c, args[0]+" server: "+args[1])
	case "login":
		return configLogin(a, c, args)
	case "model":
		if len(args) < 1 || len(args) > 2 {
			return usageErr("usage: cav config model <service> [model-id]   (no id: back to the default)")
		}
		if _, ok := services.Catalog[args[0]]; !ok {
			return usageErr("unknown service %q", args[0])
		}
		if c.Models == nil {
			c.Models = map[string]string{}
		}
		if len(args) == 1 {
			delete(c.Models, args[0])
			return saveAndReport(a, c, args[0]+" model: default")
		}
		c.Models[args[0]] = args[1]
		return saveAndReport(a, c, args[0]+" model: "+args[1])
	}
	return usageErr("unknown config command %q (show, check, set-key, unset-key, order, model, endpoint, login)", sub)
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func saveAndReport(a *app, c *services.Config, what string) error {
	if err := c.Save(); err != nil {
		return err
	}
	a.emit(map[string]any{"saved": services.Path(), "change": what}, func() {
		fmt.Printf("saved %s\n  %s\n", services.Path(), what)
	})
	return nil
}

func configShow(a *app, c *services.Config) error {
	type jobRow struct {
		Job   string   `json:"job"`
		For   string   `json:"for"`
		Order []string `json:"order"`
	}
	type keyRow struct {
		Service string `json:"service"`
		Title   string `json:"title"`
		Source  string `json:"source,omitempty"`
		Kind    string `json:"kind"`
	}
	var jobs []jobRow
	for _, j := range services.Jobs {
		jobs = append(jobs, jobRow{j, services.JobHelp[j], c.OrderFor(j)})
	}
	var keys []keyRow
	for _, n := range services.ServiceNames() {
		d := services.Catalog[n]
		k := keyRow{Service: n, Title: d.Title}
		switch d.Kind {
		case services.KindKey:
			k.Kind, k.Source = "key", c.KeySource(n)
		case services.KindEndpoint:
			k.Kind, k.Source = "server", c.KeySource(n)
		case services.KindFree:
			k.Kind = "free"
		case services.KindLocal:
			k.Kind = "local program " + d.Binary
		case services.KindBuiltin:
			k.Kind = "built in"
		}
		keys = append(keys, k)
	}
	a.emit(map[string]any{"path": services.Path(), "jobs": jobs, "services": keys}, func() {
		fmt.Printf("config: %s\n\njob order (first usable service wins):\n", services.Path())
		for _, j := range jobs {
			fmt.Printf("  %-13s %-40s %s\n", j.Job, strings.Join(j.Order, " > "), j.For)
		}
		fmt.Println("\nservices:")
		for _, k := range keys {
			src := k.Kind
			if k.Source != "" {
				src = k.Source
			}
			fmt.Printf("  %-11s %-26s %s\n", k.Service, src, k.Title)
		}
		fmt.Println("\nRun `cav config check` to test them. Keys are read only when needed and never printed.")
	})
	return nil
}

// checkServices probes the named services, or every service when names is empty.
func checkServices(c *services.Config, names []string) []services.Status {
	if len(names) == 0 {
		names = services.ServiceNames()
	}
	out := make([]services.Status, len(names))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// 1Password and the keychain may ask the user to approve each read: probe those
	// services one at a time so the prompts do not stack, and the rest in parallel.
	var wg sync.WaitGroup
	for i, n := range names {
		if _, ok := services.Catalog[n]; !ok {
			out[i] = services.Status{Service: n, State: "fail", Detail: "unknown service"}
			continue
		}
		src := c.KeySource(n)
		if strings.HasPrefix(src, "op://") || strings.HasPrefix(src, "keychain:") {
			out[i] = services.Probe(ctx, c, n)
			continue
		}
		wg.Add(1)
		go func(i int, n string) {
			defer wg.Done()
			out[i] = services.Probe(ctx, c, n)
		}(i, n)
	}
	wg.Wait()
	return out
}

func printServiceStatus(st []services.Status) {
	sort.SliceStable(st, func(i, j int) bool { return rank(st[i].State) < rank(st[j].State) })
	for _, s := range st {
		src := ""
		if s.Source != "" {
			src = " [" + s.Source + "]"
		}
		fmt.Printf("%-4s %-11s %s%s\n", s.State, s.Service, s.Detail, src)
		if s.Fix != "" && s.State != "ok" {
			fmt.Printf("     %-11s fix: %s\n", "", s.Fix)
		}
	}
}

func rank(state string) int {
	switch state {
	case "fail":
		return 0
	case "warn":
		return 1
	case "ok":
		return 2
	}
	return 3
}

func configLogin(a *app, c *services.Config, args []string) error {
	fs := flag.NewFlagSet("config login", flag.ContinueOnError)
	clientID := fs.String("client-id", "", "the OAuth client id of your API application")
	secret := fs.String("secret", "", "where the client secret lives (default: the service's key source)")
	code := fs.String("code", "", "the authorization code (asked for when missing)")
	pos, err := parseFlags(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 1 {
		return usageErr("usage: cav config login <service> --client-id ID [--secret SOURCE]")
	}
	service := pos[0]
	if _, ok := services.OAuthProviders[service]; !ok {
		return usageErr("%s has no OAuth login", service)
	}
	if *clientID == "" {
		return usageErr("give --client-id: the client id of your %s API application", service)
	}
	src := *secret
	if src == "" {
		src = c.KeySource(service)
		if src == "oauth" {
			return usageErr("give --secret: where the client secret (API key) of your application lives")
		}
	}
	u, err := services.AuthorizeURL(service, *clientID)
	if err != nil {
		return err
	}
	if *code == "" {
		if a.json {
			return usageErr("open %s, approve, then run again with --code <code>", u)
		}
		fmt.Printf("1. Open this page and approve access:\n   %s\n2. Paste the code it shows: ", u)
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		*code = strings.TrimSpace(line)
		if *code == "" {
			return usageErr("no code given")
		}
	}
	if err := services.Login(context.Background(), service, *clientID, src, *code); err != nil {
		return fail(exitError, "login failed: "+err.Error(), "")
	}
	if c.Keys == nil {
		c.Keys = map[string]string{}
	}
	c.Keys[service] = "oauth"
	return saveAndReport(a, c, service+": logged in; key source is now oauth")
}
