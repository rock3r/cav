// Package packaging holds contract tests for the plugin and install files.
package packaging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func root(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return m
}

func TestManifestsAgree(t *testing.T) {
	r := root(t)
	ap := readJSON(t, filepath.Join(r, "plugins/cavalry/plugin.json"))
	cc := readJSON(t, filepath.Join(r, "plugins/cavalry/.claude-plugin/plugin.json"))
	cx := readJSON(t, filepath.Join(r, "plugins/cavalry/.codex-plugin/plugin.json"))
	if ap["$schema"] != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" {
		t.Errorf("Agent Plugins manifest needs the 1.0.0 $schema")
	}
	// Agent Plugins has a closed field list; client-specific data goes under "extensions".
	allowed := map[string]bool{"$schema": true, "name": true, "version": true, "description": true, "author": true, "homepage": true, "repository": true, "license": true, "keywords": true, "extensions": true}
	for k := range ap {
		if !allowed[k] {
			t.Errorf("plugin.json field %q is not in the Agent Plugins 1.0.0 field list", k)
		}
	}
	if ext, ok := ap["extensions"]; ok {
		m, _ := ext.(map[string]any)
		for ns, v := range m {
			if _, obj := v.(map[string]any); !obj || !strings.Contains(ns, ".") {
				t.Errorf("extensions.%s must be an object under a reverse-domain key", ns)
			}
		}
	}
	for _, f := range []string{"name", "version", "description", "license"} {
		if ap[f] != cc[f] || ap[f] != cx[f] {
			t.Errorf("field %s differs: %v / %v / %v", f, ap[f], cc[f], cx[f])
		}
	}
	mk := readJSON(t, filepath.Join(r, ".claude-plugin/marketplace.json"))
	entry := mk["plugins"].([]any)[0].(map[string]any)
	if entry["name"] != ap["name"] || entry["source"] != "./plugins/cavalry" {
		t.Errorf("marketplace entry %v does not match the plugin", entry)
	}
	if _, ok := entry["version"]; ok {
		t.Errorf("do not set version in the marketplace entry; plugin.json owns it")
	}
}

func TestSkillFrontmatter(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(root(t), "plugins/cavalry/skills/cavalry/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	m := regexp.MustCompile(`(?s)^---\n(.*?)\n---\n`).FindStringSubmatch(s)
	if m == nil {
		t.Fatal("SKILL.md has no frontmatter")
	}
	fm := m[1]
	if !strings.Contains(fm, "\nname: cavalry\n") && !strings.HasPrefix(fm, "name: cavalry\n") {
		t.Error("frontmatter name must be cavalry (the folder name)")
	}
	d := regexp.MustCompile(`(?m)^description: (.*)$`).FindStringSubmatch(fm)
	if d == nil || len(d[1]) == 0 || len(d[1]) > 1024 {
		t.Errorf("description must be 1-1024 characters")
	}
	for _, line := range strings.Split(fm, "\n") {
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		key := strings.SplitN(line, ":", 2)[0]
		switch key {
		case "name", "description", "license", "compatibility", "metadata", "allowed-tools":
		default:
			t.Errorf("frontmatter key %q is not portable (Agent Skills spec)", key)
		}
	}
	if n := strings.Count(s, "\n"); n > 500 {
		t.Errorf("SKILL.md has %d lines; keep it under 500", n)
	}
	// Every referenced file exists.
	for _, ref := range regexp.MustCompile("`(references/[a-z-]+\\.md)`").FindAllStringSubmatch(s, -1) {
		if _, err := os.Stat(filepath.Join(root(t), "plugins/cavalry/skills/cavalry", ref[1])); err != nil {
			t.Errorf("SKILL.md points at missing %s", ref[1])
		}
	}
}

func TestInstallScriptCopiesMatch(t *testing.T) {
	r := root(t)
	for _, f := range []string{"install.sh", "install.ps1"} {
		a, _ := os.ReadFile(filepath.Join(r, f))
		b, _ := os.ReadFile(filepath.Join(r, "plugins/cavalry/skills/cavalry/scripts", f))
		if len(a) == 0 || !bytes.Equal(a, b) {
			t.Errorf("%s and the skill's copy differ; copy the root file into skills/cavalry/scripts/", f)
		}
	}
}

// Codex rules from developers.openai.com/plugins/deploy/submission (checked 2026-09-30).
func TestCodexInterface(t *testing.T) {
	r := root(t)
	// OpenAI reads extensions."com.openai" in the root plugin.json. Older Codex versions read
	// .codex-plugin/plugin.json instead, so it keeps an identical copy of the interface.
	ap := readJSON(t, filepath.Join(r, "plugins/cavalry/plugin.json"))
	ext, _ := ap["extensions"].(map[string]any)
	oa, _ := ext["com.openai"].(map[string]any)
	ui, _ := oa["interface"].(map[string]any)
	if ui == nil {
		t.Fatal(`plugin.json has no extensions."com.openai".interface`)
	}
	cx := readJSON(t, filepath.Join(r, "plugins/cavalry/.codex-plugin/plugin.json"))
	a, _ := json.Marshal(ui)
	b, _ := json.Marshal(cx["interface"])
	if !bytes.Equal(a, b) {
		t.Errorf(".codex-plugin/plugin.json interface differs from plugin.json extensions.\"com.openai\".interface")
	}
	categories := map[string]bool{"Productivity": true, "Creativity": true, "Developer Tools": true, "Business & Operations": true,
		"Data & Analytics": true, "Communication": true, "Education & Research": true, "Security": true, "Finance": true,
		"Healthcare": true, "Travel": true, "Entertainment": true, "Other": true}
	if c, _ := ui["category"].(string); !categories[c] {
		t.Errorf("interface.category %q is not an allowed Codex category", c)
	}
	for _, f := range []string{"displayName", "shortDescription", "longDescription", "developerName", "capabilities", "composerIcon", "logo"} {
		if _, ok := ui[f]; !ok {
			t.Errorf("interface.%s is required for Codex", f)
		}
	}
	if s, _ := ui["shortDescription"].(string); len(s) > 30 {
		t.Errorf("interface.shortDescription is %d characters; the directory allows 30", len(s))
	}
	for _, f := range []string{"composerIcon", "logo"} {
		p, _ := ui[f].(string)
		if !strings.HasPrefix(p, "./assets/") {
			t.Errorf("interface.%s must point under ./assets/, got %q", f, p)
			continue
		}
		if _, err := os.Stat(filepath.Join(r, "plugins/cavalry", p)); err != nil {
			t.Errorf("interface.%s: %v", f, err)
		}
	}
	mk := readJSON(t, filepath.Join(r, ".agents/plugins/marketplace.json"))
	for _, e := range mk["plugins"].([]any) {
		entry := e.(map[string]any)
		if c, _ := entry["category"].(string); !categories[c] {
			t.Errorf("marketplace entry %v: category %q is not an allowed Codex category", entry["name"], c)
		}
	}
}
