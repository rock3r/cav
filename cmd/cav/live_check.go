package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rock3r/cav/internal/imagegen"
	"github.com/rock3r/cav/internal/music"
	"github.com/rock3r/cav/internal/services"
)

// liveCheck is one small paid call that proves a service can do its real job, not only that
// the key is accepted. Costs are rough list prices (2026-10) and only shown to the user.
type liveCheck struct {
	cost string
	run  func(ctx context.Context, c *services.Config, ch *services.Choice) (string, error)
}

func imageLive(ctx context.Context, c *services.Config, ch *services.Choice) (string, error) {
	im, err := imagegen.Generate(ctx, c, ch, imagegen.Request{Prompt: "a single small blue dot on white, flat", Aspect: "1:1"})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("made a %d-byte %s with %s", len(im.Data), im.MIME, im.Model), nil
}

var liveChecks = map[string]liveCheck{
	"gemini":     {"about $0.04 (one image)", imageLive},
	"openai":     {"about $0.01-0.05 (one image)", imageLive},
	"openrouter": {"about $0.04 (one image)", imageLive},
	"recraft":    {"about $0.01-0.04 (one image)", imageLive},
	"fal":        {"about $0.01-0.03 (one image)", imageLive},
	"elevenlabs": {"a few credits (a half-second sound effect)", func(ctx context.Context, c *services.Config, ch *services.Choice) (string, error) {
		tr, err := music.SoundEffect(ctx, ch.Key, "a short soft click", 0.5, false)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("made a %d-byte sound effect", len(tr.Data)), nil
	}},
}

// runLiveChecks runs the paid checks for the services that passed the free check. It asks
// first unless yes is set; with --json, yes is required.
func runLiveChecks(a *app, c *services.Config, st []services.Status, yes bool) ([]services.Status, error) {
	var todo []string
	for _, s := range st {
		if _, ok := liveChecks[s.Service]; ok && s.State == "ok" {
			todo = append(todo, s.Service)
		}
	}
	if len(todo) == 0 {
		return nil, nil
	}
	if !yes {
		if a.json {
			return nil, usageErr("--live spends money: add --yes to confirm")
		}
		fmt.Fprintln(os.Stderr, "--live makes one small paid call per service:")
		for _, n := range todo {
			fmt.Fprintf(os.Stderr, "  %-11s %s\n", n, liveChecks[n].cost)
		}
		fmt.Fprint(os.Stderr, "Go ahead? [y/N] ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if !strings.EqualFold(strings.TrimSpace(line), "y") {
			return nil, fail(exitError, "live checks cancelled", "")
		}
	}
	var out []services.Status
	for _, n := range todo {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		res := services.Status{Service: n, Job: "live"}
		ch, err := services.Pick(ctx, c, liveJob(n), n)
		if err == nil {
			res.Detail, err = liveChecks[n].run(ctx, c, ch)
		}
		cancel()
		if err != nil {
			res.State, res.Detail = "fail", err.Error()
		} else {
			res.State = "ok"
		}
		out = append(out, res)
	}
	return out, nil
}

func liveJob(service string) string {
	if service == "elevenlabs" {
		return "sfx"
	}
	if service == "recraft" {
		return "image.vector"
	}
	return "image"
}
