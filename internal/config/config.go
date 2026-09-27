// Package config holds the file locations that the cav CLI and the Cavalry bridge share.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	DefaultHost = "127.0.0.1"
	DefaultPort = 8723
	BridgeFile  = "cav-bridge.js"
)

// Home is the cav state folder: ~/.cav. The bridge computes the same path with
// api.getHomeFolder(), so the two sides agree without any configuration.
func Home() string {
	if v := os.Getenv("CAV_HOME"); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ".cav"
	}
	return filepath.Join(h, ".cav")
}

func TokenPath() string   { return filepath.Join(Home(), "token") }
func JobsDir() string     { return filepath.Join(Home(), "jobs") }
func CacheDir() string    { return filepath.Join(Home(), "cache") }
func DocsDir() string     { return filepath.Join(CacheDir(), "docs") }
func PreloadPath() string { return filepath.Join(Home(), "helpers.js") }

// SpoolDir returns the spool folder when the CLI must not use the network
// (for example inside a sandbox that blocks loopback). Empty means HTTP.
func SpoolDir() string { return os.Getenv("CAV_SPOOL") }

func Host() string {
	if v := os.Getenv("CAV_BRIDGE_HOST"); v != "" {
		return v
	}
	return DefaultHost
}

func Port() int {
	if v := os.Getenv("CAV_BRIDGE_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			return p
		}
	}
	return DefaultPort
}

// ScriptsDir is Cavalry's per-user Scripts folder.
func ScriptsDir() string {
	if v := os.Getenv("CAV_SCRIPTS_DIR"); v != "" {
		return v
	}
	h, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(h, "Library", "Application Support", "Cavalry", "Scripts")
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata == "" {
			appdata = filepath.Join(h, "AppData", "Roaming")
		}
		return filepath.Join(appdata, "Cavalry", "Scripts")
	default:
		return filepath.Join(h, ".local", "share", "Cavalry", "Scripts")
	}
}

// ReadToken returns the shared secret, or an error that tells the user what to run.
func ReadToken() (string, error) {
	b, err := os.ReadFile(TokenPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("token file %s is missing: run `cav setup`", TokenPath())
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// EnsureToken creates the token file if it does not exist. It returns true when it created one.
func EnsureToken() (bool, error) {
	if err := os.MkdirAll(Home(), 0o700); err != nil {
		return false, err
	}
	if _, err := os.Stat(TokenPath()); err == nil {
		return false, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return false, err
	}
	f, err := os.OpenFile(TokenPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.WriteString(hex.EncodeToString(buf))
	return err == nil, err
}
