package flow

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// foldCase makes names match without regard to case, as the file systems of Windows and
// macOS do. It is a variable so that tests cover both ways.
var foldCase = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

var errFound = errors.New("found")

// Check says whether an output of a stage exists in the project folder root. output is one
// entry of a stage's outputs, as the parser returns it:
//
//   - a plain path is a regular file;
//   - a path with a trailing / is a folder that holds at least one regular file, at any depth,
//     whose name does not start with a dot;
//   - a path with * is a pattern. A * matches inside one folder, never across /, and never a
//     name that starts with a dot unless the pattern starts with one.
//
// Review files (name.review.json) and the review/ folder of snapshots are not outputs, so
// they never match. matches lists what was found, relative to root with /, in order. Check
// never writes.
func Check(root, output string) (matches []string, ok bool, err error) {
	wantDir := strings.HasSuffix(output, "/")
	segs := strings.Split(strings.Trim(output, "/"), "/")
	if len(segs) == 0 || segs[0] == "" {
		return nil, false, nil
	}
	if err := walkSegments(root, "", segs, wantDir, &matches); err != nil {
		return nil, false, err
	}
	sort.Strings(matches)
	return matches, len(matches) > 0, nil
}

// walkSegments expands segs below dir (relative to root, with /).
func walkSegments(root, dir string, segs []string, wantDir bool, out *[]string) error {
	seg, rest := segs[0], segs[1:]
	last := len(rest) == 0
	names, err := candidates(filepath.Join(root, filepath.FromSlash(dir)), seg)
	if err != nil {
		return err
	}
	for _, name := range names {
		rel := name
		if dir != "" {
			rel = dir + "/" + name
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		st, err := os.Stat(full)
		if err != nil {
			continue // a broken link, or gone since the listing
		}
		switch {
		case !last:
			if st.IsDir() {
				if err := walkSegments(root, rel, rest, wantDir, out); err != nil {
					return err
				}
			}
		case wantDir:
			if st.IsDir() {
				if has, err := holdsFile(full); err != nil {
					return err
				} else if has {
					*out = append(*out, rel+"/")
				}
			}
		case st.Mode().IsRegular():
			*out = append(*out, rel)
		}
	}
	return nil
}

// candidates lists the names in dir that match one path segment. A segment without * names
// one entry; it is listed (not just checked) only when case is folded.
func candidates(dir, seg string) ([]string, error) {
	star := strings.Contains(seg, "*")
	if !star && !foldCase {
		if _, err := os.Lstat(filepath.Join(dir, seg)); err != nil {
			return nil, nil
		}
		return []string{seg}, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid) || isNotDir(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if star && skipped(name, seg, e.IsDir()) {
			continue
		}
		if star {
			if !starMatch(seg, name) {
				continue
			}
		} else if !sameName(seg, name) {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

// isNotDir reports the error of listing something that is a file.
func isNotDir(err error) bool {
	var pe *fs.PathError
	return errors.As(err, &pe) && (strings.Contains(pe.Err.Error(), "not a directory") || strings.Contains(pe.Err.Error(), "cannot find the path"))
}

// skipped says whether a pattern must not match this entry: a name with a dot in front
// that the pattern does not ask for, a review file, or the folder of review snapshots.
func skipped(name, pattern string, isDir bool) bool {
	if strings.HasPrefix(name, ".") && !strings.HasPrefix(pattern, ".") {
		return true
	}
	return isReviewArtifact(name, isDir)
}

func isReviewArtifact(name string, isDir bool) bool {
	if isDir {
		return name == "review"
	}
	return strings.HasSuffix(name, ".review.json")
}

func sameName(a, b string) bool {
	if foldCase {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// starMatch matches name against a pattern in which only * is special (zero or more
// characters). ? and [ are plain characters.
func starMatch(pattern, name string) bool {
	if foldCase {
		pattern, name = strings.ToLower(pattern), strings.ToLower(name)
	}
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == name
	}
	if !strings.HasPrefix(name, parts[0]) {
		return false
	}
	name = name[len(parts[0]):]
	last := parts[len(parts)-1]
	for _, p := range parts[1 : len(parts)-1] {
		i := strings.Index(name, p)
		if i < 0 {
			return false
		}
		name = name[i+len(p):]
	}
	return strings.HasSuffix(name, last) && len(name) >= len(last)
}

// holdsFile reports whether a folder has a regular file anywhere inside it, ignoring hidden
// files and folders, review files and the review/ folder.
func holdsFile(dir string) (bool, error) {
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == dir {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") || isReviewArtifact(name, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			return errFound
		}
		return nil
	})
	if errors.Is(err, errFound) {
		return true, nil
	}
	return false, err
}
