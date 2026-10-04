package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/rock3r/cav/assets"
	"github.com/rock3r/cav/internal/fontmeta"
)

var sourceOnce sync.Once
var preloadSource []byte

func helperSource() []byte {
	sourceOnce.Do(func() {
		home, _ := os.UserHomeDir()
		roots := filepath.SplitList(os.Getenv("CAV_FONT_DIR"))
		switch runtime.GOOS {
		case "darwin":
			roots = append(roots, filepath.Join(home, "Library", "Fonts"), "/Library/Fonts", "/System/Library/Fonts")
		case "windows":
			roots = append(roots, filepath.Join(os.Getenv("WINDIR"), "Fonts"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts"))
		default:
			roots = append(roots, filepath.Join(home, ".local", "share", "fonts"), "/usr/share/fonts", "/usr/local/share/fonts")
		}
		data, _ := json.Marshal(fontmeta.Discover(roots))
		preloadSource = append([]byte("globalThis.CAV_FONT_AXES="+string(data)+";\n"), assets.HelpersJS...)
	})
	return preloadSource
}
