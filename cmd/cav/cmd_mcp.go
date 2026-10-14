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
		summary: "Run a small MCP server (stdio) that shows the review page (video or stills) inside the chat, as an MCP App.",
		run:     cmdMCP,
	})
	longHelp["mcp"] = `
cav mcp runs an MCP server on stdin and stdout. The cavalry plugin for Claude Code and Codex
starts it for you. Without the plugin, register it with your agent:
  Claude Code:  claude mcp add cav -- cav mcp
  Codex:        codex mcp add cav -- cav mcp
It has two tools for the agent:
  show_review [path]   shows the cav review page of a render, brief.json, moodboard.json or
                       storyboard.json inside the chat, as an MCP App. For a render, the
                       person can play it, draw and leave notes. For stills, the page shows
                       a grid of items: the person gives a verdict on each, leaves notes,
                       and approves the stage. Both have "Send to agent", as in the browser.
  review_notes [path]  returns the sent notes, with a snapshot image of each, and marks
                       the send received, as cav review wait does. When the person approved
                       a stage, it says so. With nothing sent, it returns the open notes.
                       For chats without a shell.
Only the person approves a stage. The tools give the agent no way to approve.
Notes go to the same <name>.review.json.

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
		Nonce:    sharedReviewNonce(),
		CacheDir: filepath.Join(config.CacheDir(), "review-preview"),
		Resolve: func(video string) (string, error) {
			var pos []string
			if video != "" {
				pos = []string{video}
			}
			return reviewTarget(pos)
		},
	})
	return srv.Run(ctx, &mcp.StdioTransport{})
}
