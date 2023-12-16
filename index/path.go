package index

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Path is a non-empty, unrooted, slash-separated path. Except for the special
// "." root, the path ends in a '/' if it refers to a directory.
type Path struct{ p string }

// Root is the special "." path.
var Root = Path{"."}

func filePath(p string) Path {
	if path.Clean(filepath.ToSlash(p)) != p {
		panic(fmt.Sprintf("index: non-clean file path: %q", p))
	}
	return Path{p}
}

func (p Path) String() string {
	return p.p
}

func (p Path) IsDir() bool {
	return p.p == "." || p.p[len(p.p)-1] == '/'
}

// Dir returns the parent directory of p.
func (p Path) Dir() Path {
	switch i := strings.LastIndexByte(p.p, '/'); i {
	case -1:
		return Path{"."}
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

func (p Path) Contains(other Path) bool {
	return p.p == "." || (strings.HasPrefix(other.p, p.p) && p.p[len(p.p)-1] == '/')
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
		if ca, cb := a[i], b[i]; ca != cb {
			// Directory is less than a file
			if aDir := isDir(a[i:]); aDir != isDir(b[i:]) {
				return aDir
			}
			// Path separator is less than any other byte
			return ca == '/' || (cb != '/' && ca < cb)
		}
	}
	// One of the paths is a prefix of the other. If needed, swap the paths so
	// that a is a prefix of b to simplify the logic.
	invert := false
	if len(a) >= len(b) {
		if len(a) == len(b) {
			return false // Paths are identical
		}
		a, b = b, a
		invert = true
	}
	// a is a prefix of b and the next byte in b cannot be a '/'. We require
	// directories to end with a '/' to ensure consistent ordering when sorting
	// ["b/", "b/c", "a"]. Without the terminal '/', we'd sort "a" before "b"
	// since we wouldn't know that "b" is a directory.
	bSep := strings.IndexByte(b[len(a):], '/')
	if bSep == 0 {
		panic(fmt.Sprintf("index: directory without separator suffix: %s", a))
	}
	// If a ends with '/', then it's a parent of b. If b does not have any more
	// separators, then a and b are regular files in the same directory and a is
	// shorter. Otherwise, a is a file and b is a directory.
	return (a[len(a)-1] == '/' || bSep < 0) != invert
}

func isDir(s string) bool {
	return strings.IndexByte(s, '/') >= 0
}
