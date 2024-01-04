package index

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Path is a non-empty, unrooted, clean, slash-separated path. Except for the
// special "." root, the path ends in a '/' if it refers to a directory.
type Path struct{ p string }

// Root is the special "." path.
var Root = Path{"."}

// filePath wraps a path to a file. It panics if the path is not a file, not
// slash-separated, or not clean.
func filePath(p string) Path {
	if path.Clean(filepath.ToSlash(p)) != p {
		panic(fmt.Sprintf("index: non-clean or non-file path: %s", p))
	}
	if p == "." {
		panic("index: non-file path: .")
	}
	return Path{p}
}

// String returns the raw path.
func (p Path) String() string {
	return p.p
}

// IsDir returns whether the path refers to a directory.
func (p Path) IsDir() bool {
	return p.p == "." || (0 < len(p.p) && p.p[len(p.p)-1] == '/')
}

// Contains returns whether other is under the directory tree p. It returns true
// if the paths are equal or if p is ".".
func (p Path) Contains(other Path) bool {
	return p.p == "." || (0 < len(p.p) && len(p.p) <= len(other.p) &&
		other.p[:len(p.p)] == p.p && p.p[len(p.p)-1] == '/')
}

// Dir returns the parent directory of p.
func (p Path) Dir() Path {
	switch i := strings.LastIndexByte(p.p[:len(p.p)-1], '/'); i {
	case -1:
		return Root
	case 0:
		panic(fmt.Sprintf("index: rooted path: %s", p))
	default:
		return Path{p.p[:i+1]}
	}
}

// Base returns the last element of p.
func (p Path) Base() string {
	return path.Base(p.p)
}

// less returns whether path p should be sorted before other. Directories are
// sorted before files.
func (p Path) less(other Path) bool {
	a, b := p.p, other.p
	if a == "." || b == "." {
		return a == "." && b != "." // Root is less than all other paths
	}
	// Find the first byte mismatch
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			// Directory is less than a file
			aDir := strings.IndexByte(a[i:], '/') >= 0
			if aDir != (strings.IndexByte(b[i:], '/') >= 0) {
				return aDir
			}
			// Path separator is less than any other byte
			return a[i] == '/' || (b[i] != '/' && a[i] < b[i])
		}
	}
	// One of the paths is a prefix of the other. If needed, swap the paths so
	// that a is a prefix of b to simplify the remaining logic.
	invert := false
	if len(a) >= len(b) {
		if len(a) == len(b) {
			return false // Same path
		}
		a, b = b, a
		invert = true
	}
	// a is a prefix of b and the next byte in b cannot be a '/' since the same
	// name cannot be both a file and a directory. We require directories to end
	// with a '/' to ensure consistent ordering when sorting ["b/", "b/c", "a"].
	// Without the '/' suffix, we'd sort "a" before "b" since we wouldn't know
	// that "b" is a directory.
	bSep := strings.IndexByte(b[len(a):], '/')
	if bSep == 0 {
		panic(fmt.Sprintf("index: directory without separator suffix: %s", a))
	}
	// If a ends with '/', then it's a parent of b. If b does not have any more
	// separators, then a and b are regular files in the same directory and a is
	// shorter. Otherwise, a is a file and b is a directory.
	return (a[len(a)-1] == '/' || bSep < 0) != invert
}
