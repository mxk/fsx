package index

import (
	"cmp"
	"fmt"
	stdpath "path"
	"path/filepath"
	"strings"
)

// path is an unrooted, clean, slash-separated path. An empty path is invalid.
// Except for the special "." root, a directory path always ends with a '/'.
type path struct{ p string }

// root is the special "." path.
var root = path{"."}

// dirPath creates a directory path. It panics if the path is invalid.
func dirPath(p string) path {
	if c := cleanPath(p); c != "" {
		if c != "." && c[len(c)-1] != '/' {
			c += "/"
		}
		return path{c}
	}
	panic(fmt.Sprint("index: invalid path: ", p))
}

// filePath creates a file path. It panics if the path is invalid.
func filePath(p string) path {
	if c := (path{cleanPath(p)}); c.isFile() {
		return c
	}
	panic(fmt.Sprint("index: invalid or non-file path: ", p))
}

// strictFilePath creates a file path. It panics if the path is not identical to
// that returned by filePath.
func strictFilePath(p string) path {
	if p == filePath(p).p {
		return path{p}
	}
	panic(fmt.Sprint("index: non-clean file path: ", p))
}

// anyPath returns the directory and/or file interpretations of path p,
// depending on which one is possible.
func anyPath(p string) (dir, file path) {
	if p = cleanPath(p); p != "" {
		if p == "." || p[len(p)-1] == '/' {
			dir = path{p}
		} else {
			p += "/"
			dir, file = path{p}, path{p[:len(p)-1]}
		}
	}
	return
}

// String returns the raw path.
func (p path) String() string { return p.p }

// contains returns whether other is under the directory tree p. It returns true
// if the paths are equal (same directory) or if p is ".".
func (p path) contains(other path) bool {
	return p.p == "." || (0 < len(p.p) && len(p.p) <= len(other.p) &&
		other.p[:len(p.p)] == p.p && p.p[len(p.p)-1] == '/')
}

// Dir returns the parent directory of p.
func (p path) dir() path {
	i := strings.LastIndexByte(p.p[:len(p.p)-1], '/')
	if i > 0 {
		return path{p.p[:i+1]}
	}
	if i < 0 {
		if p.isEmpty() {
			return path{}
		}
		return root
	}
	panic(fmt.Sprint("index: rooted path: ", p))
}

// base returns the last element of p.
func (p path) base() string {
	if p.isEmpty() {
		return ""
	}
	return stdpath.Base(p.p)
}

// commonRoot returns the path that is a parent of both p and other.
func (p path) commonRoot(other path) path {
	a, b := p.p, other.p
	for {
		i := strings.IndexByte(a, '/')
		if i < 0 || i != strings.IndexByte(b, '/') || a[:i] != b[:i] {
			if s := p.p[:len(p.p)-len(a)]; s != "" {
				return path{s}
			}
			return root
		}
		a, b = a[i+1:], b[i+1:]
	}
}

// dist returns the distance between two paths in terms of directories traversed
// to go from one to the other.
func (p path) dist(other path) int {
	if r := p.commonRoot(other); r != root {
		p.p, other.p = p.p[len(r.p):], other.p[len(r.p):]
	}
	return strings.Count(p.p, "/") + strings.Count(other.p, "/")
}

// isEmpty returns whether p is an invalid empty path.
func (p path) isEmpty() bool { return len(p.p) == 0 }

// isDir returns whether the path refers to a directory.
func (p path) isDir() bool {
	return 0 < len(p.p) && (p.p == "." || p.p[len(p.p)-1] == '/')
}

// isFile returns whether the path refers to a file.
func (p path) isFile() bool {
	return 0 < len(p.p) && p.p != "." && p.p[len(p.p)-1] != '/'
}

// cmp returns -1 if p < other, 0 if p == other, and +1 if p > other.
// Directories are considered less than files. It panics if either path is empty
// or if the same name refers to both a file and a directory.
func (p path) cmp(other path) int {
	lessIf := func(b bool) int {
		if b {
			return -1
		}
		return +1
	}
	a, b := p.p, other.p
	if a == "." || b == "." {
		if a == b {
			return 0
		}
		if a == "" || b == "" {
			panic("index: empty path")
		}
		return lessIf(a == ".") // Root is less than all other paths
	}
	// Find the first byte mismatch
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			// Directory is less than a file
			aDir := strings.IndexByte(a[i:], '/') >= 0
			if aDir != (strings.IndexByte(b[i:], '/') >= 0) {
				return lessIf(aDir)
			}
			// Path separator is less than any other byte
			if a[i] != '/' && b[i] != '/' {
				return cmp.Compare(a[i], b[i])
			}
			return lessIf(a[i] == '/')
		}
	}
	// One of the paths is a prefix of the other. If needed, swap the paths so
	// that a is a prefix of b to simplify the remaining logic.
	invert := false
	if len(a) >= len(b) {
		if len(a) == len(b) {
			return 0 // Same path
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
		panic(fmt.Sprint("index: directory without separator suffix: ", a))
	}
	if a == "" {
		panic("index: empty path")
	}
	// If a ends with '/', then it's a parent of b. If b does not have any more
	// separators, then a and b are regular files in the same directory and a is
	// shorter. Otherwise, a is a file and b is a directory.
	return lessIf((a[len(a)-1] == '/' || bSep < 0) != invert)
}

// steps iterates over every step in a path.
type steps struct {
	path
	n int
}

// next returns the next step in s or (path{}, false) if the last step was
// reached. It does not return the root directory.
func (s *steps) next() (path, bool) {
	if s.n >= len(s.p) || s.path == root {
		return path{}, false
	}
	if i := strings.IndexByte(s.p[s.n:], '/'); i > 0 {
		s.n += i + 1
	} else if s.n = len(s.p); i == 0 {
		panic(fmt.Sprint("index: rooted or non-clean path: ", s)) // Shouldn't happen
	}
	return path{s.p[:s.n]}, true
}

// skip causes next to return the step after p if p is a parent of the final
// path (p.contains(s.path) == true) and hasn't been returned yet.
func (s *steps) skip(p path) {
	if s.n < len(p.p) && len(p.p) <= len(s.p) &&
		s.p[:len(p.p)] == p.p && p.p[len(p.p)-1] == '/' {
		s.n = len(p.p)
	}
}

// uniqueDirs visits all unique directories in a set of paths.
type uniqueDirs []steps

// add adds path p to the set.
func (u *uniqueDirs) add(p path) { *u = append(*u, steps{path: p}) }

// clear removes all paths from the set.
func (u *uniqueDirs) clear() { *u = (*u)[:0] }

// forEach calls fn for each unique directory in the set, leaving the set empty.
// For example, if the set contains paths "A/B/", "A/C/", and "D/", fn will be
// called for ".", "A/", "A/B/", "A/C/", and "D/".
func (u *uniqueDirs) forEach(fn func(path)) {
	defer u.clear()
	if len(*u) > 0 {
		fn(root)
	}
	for len(*u) > 0 {
		if p, ok := (*u)[0].next(); ok {
			fn(p)
			for i := 1; i < len(*u); i++ {
				(*u)[i].skip(p)
			}
		} else {
			*u = append((*u)[:0], (*u)[1:]...)
		}
	}
}

// cleanPath returns a clean, slash-separated representation of p. It returns ""
// if p is invalid. The trailing separator, if present, is preserved.
func cleanPath(p string) string {
	// VolumeName != "" is a superset of filepath.IsAbs test on Windows
	if p == "" || filepath.VolumeName(p) != "" {
		return ""
	}
	// path.Clean is more efficient than filepath.Clean and we want to return ""
	// for cases handled by filepath.postClean.
	p = filepath.ToSlash(p)
	c := stdpath.Clean(p)
	if stdpath.IsAbs(c) || (len(c) >= 2 && (c[1] == ':' || c[:2] == "..")) {
		return ""
	}
	// Preserve the trailing '/' and pointer values, if possible
	if c == "." {
		c = root.p
	} else if p[len(p)-1] == '/' {
		if len(p) == len(c)+1 && p[:len(c)] == c {
			c = p
		} else {
			c += "/"
		}
	}
	return c
}
