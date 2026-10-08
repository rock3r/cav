package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/config"
	"github.com/rock3r/cav/internal/reviewmcp"
)

func init() {
	register(command{
		name:    "mcp",
		args:    "",
		summary: "Run a small MCP server (stdio) that shows the review page inside the chat, as an MCP App.",
		run:     cmdMCP,
	})
	longHelp["mcp"] = `
cav mcp runs an MCP server on stdin and stdout. The cavalry plugin for Claude Code and Codex
starts it for you. Without the plugin, register it with your agent:
  Claude Code:  claude mcp add cav -- cav mcp
  Codex:        codex mcp add cav -- cav mcp
It has one tool for the agent, show_review [video], which shows the cav review page of a
render inside the chat as an MCP App. The person can play it, draw and leave notes, and
press "Send to agent", as in the browser. Notes go to the same <video>.review.json.

The chat frame is small and has no network, so the page plays a preview copy (at most 640
pixels wide, cached in ~/.cav/cache/review-preview) that the server sends inline, up to
24 MB. It suits a quick look: for frame-by-frame review run cav review in a shell.
Not every host draws MCP Apps: the chat of the Claude desktop app does, its Code tab does
not. Hosts that do not still get the list of open notes.`
}

func cmdMCP(a *app, args []string) error {
	if len(args) > 0 {
		return usageErr("cav mcp takes no arguments")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := reviewmcp.New(reviewmcp.Options{
		Version:  version,
		Page:     assets.ReviewPage,
		Shim:     assets.ReviewAppShim,
		Author:   defaultAuthor(),
		CacheDir: filepath.Join(config.CacheDir(), "review-preview"),
		Resolve: func(video string) (string, error) {
			var pos []string
			if video != "" {
				pos = []string{video}
			}
			return reviewVideo(pos)
		},
	})
	return srv.Run(ctx, &mcp.StdioTransport{})
}
