package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// macBundleRoot validates archive paths before native tar restores signature/ticket metadata.
// Cav bundles contain only directories and regular files (AppleDouble metadata is regular).
func macBundleRoot(archive []byte) (string, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	root := ""
	executable := false
	total := int64(0)
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			return "", e
		}
		name := strings.TrimPrefix(h.Name, "./")
		clean := path.Clean(name)
		if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(name, "\\") {
			return "", fmt.Errorf("unsafe archive path %q", h.Name)
		}
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA && h.Typeflag != tar.TypeDir {
			return "", fmt.Errorf("unsupported archive entry %q", h.Name)
		}
		total += h.Size
		if total > 200<<20 {
			return "", fmt.Errorf("archive expands beyond 200 MiB")
		}
		parts := strings.Split(clean, "/")
		if len(parts) >= 2 && parts[1] == "Cav.app" {
			if !strings.HasPrefix(parts[0], "cav-cli-") {
				return "", fmt.Errorf("unexpected bundle root")
			}
			candidate := parts[0] + "/Cav.app"
			if root != "" && root != candidate {
				return "", fmt.Errorf("multiple bundles")
			}
			root = candidate
			if clean == candidate+"/Contents/MacOS/cav" {
				if h.Mode&0111 == 0 {
					return "", fmt.Errorf("launcher is not executable")
				}
				executable = true
			}
		}
	}
	if root == "" || !executable {
		return "", fmt.Errorf("complete executable Cav.app not found; refusing bare-binary downgrade")
	}
	return root, nil
}
func verifyMacApp(app string) error {
	commands := [][]string{{"codesign", "--verify", "--deep", "--strict", app}, {"xcrun", "stapler", "validate", app}, {"spctl", "--assess", "--type", "execute", app}}
	for _, args := range commands {
		if out, e := exec.Command(args[0], args[1:]...).CombinedOutput(); e != nil {
			return fmt.Errorf("%s failed: %s: %w", args[0], out, e)
		}
	}
	for _, target := range []string{app, filepath.Join(app, "Contents", "MacOS", "cav")} {
		out, e := exec.Command("codesign", "-d", "--verbose=4", target).CombinedOutput()
		if e != nil {
			return e
		}
		text := string(out)
		for _, required := range []string{"Authority=Developer ID Application:", "Timestamp=", "(runtime)", "TeamIdentifier="} {
			if !strings.Contains(text, required) {
				return fmt.Errorf("release signature missing %s", required)
			}
		}
	}
	return nil
}
func macInstallDir(self string) (string, error) {
	if filepath.Base(filepath.Dir(self)) != "MacOS" {
		return filepath.Dir(self), nil
	} // older bare install
	app := filepath.Dir(filepath.Dir(filepath.Dir(self)))
	if filepath.Base(app) != "Cav.app" {
		return "", fmt.Errorf("unexpected executable location")
	}
	bundles := filepath.Dir(filepath.Dir(app))
	if filepath.Base(bundles) != ".cav-bundles" {
		return "", fmt.Errorf("manually installed bundles require install.sh with CAV_BIN_DIR")
	}
	return filepath.Dir(bundles), nil
}
func installMacBundle(archive []byte, self string) (string, error) {
	root, err := macBundleRoot(archive)
	if err != nil {
		return "", err
	}
	binDir, err := macInstallDir(self)
	if err != nil {
		return "", err
	}
	bundles := filepath.Join(binDir, ".cav-bundles")
	if err = os.MkdirAll(bundles, 0755); err != nil {
		return "", err
	}
	stage, err := os.MkdirTemp(bundles, "install.")
	if err != nil {
		return "", err
	}
	// Failure removes only the new, never-published tree. Previous installs are retained.
	published := false
	defer func() {
		if !published {
			os.RemoveAll(stage)
		}
	}()
	archivePath := filepath.Join(stage, "download.tar.gz")
	if err = os.WriteFile(archivePath, archive, 0600); err != nil {
		return "", err
	}
	if out, e := exec.Command("/usr/bin/tar", "-xzf", archivePath, "-C", stage).CombinedOutput(); e != nil {
		return "", fmt.Errorf("extract failed: %s: %w", out, e)
	}
	app := filepath.Join(stage, filepath.FromSlash(root))
	if err = verifyMacApp(app); err != nil {
		return "", err
	}
	installedApp := filepath.Join(stage, "Cav.app")
	if err = os.Rename(app, installedApp); err != nil {
		return "", err
	}
	app = installedApp
	if err = verifyMacApp(app); err != nil {
		return "", err
	}
	launcher := filepath.Join(app, "Contents", "MacOS", "cav")
	if st, e := os.Stat(launcher); e != nil || st.Mode()&0111 == 0 {
		return "", fmt.Errorf("installed launcher is not executable")
	}
	// Stage symlink on the same filesystem for atomic replacement. Do not mutate the signed app.
	link := filepath.Join(stage, "path-link")
	if err = os.Symlink(launcher, link); err != nil {
		return "", err
	}
	if err = os.Rename(link, filepath.Join(binDir, "cav")); err != nil {
		return "", err
	}
	published = true
	_ = os.Remove(archivePath)
	return launcher, nil
}
