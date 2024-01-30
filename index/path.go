package index

import (
	"fmt"
	stdpath "path"
	"path/filepath"
	"strings"
)

// emptyPath is a panic message used when a path is empty.
const emptyPath = "index: empty path"

// path is an unrooted, clean, slash-separated path. An empty path is invalid.
// Except for the root ("."), a directory path always ends with a '/'.
type path string

// dirPath creates a directory path. It panics if the path is invalid.
func dirPath(p string) path {
	if c := cleanPath(p); c != "" {
		if c != "." && c[len(c)-1] != '/' {
			c += "/"
		}
		return path(c)
	}
	panic(fmt.Sprint("index: invalid path: ", p))
}

// filePath creates a file path. It panics if the path is invalid.
func filePath(p string) path {
	if c := path(cleanPath(p)); c.isFile() {
		return c
	}
	panic(fmt.Sprint("index: invalid or non-file path: ", p))
}

// strictFilePath creates a file path. It panics if the path is not identical to
// that returned by filePath.
func strictFilePath(p string) path {
	if p == string(filePath(p)) {
		return path(p)
	}
	panic(fmt.Sprint("index: non-clean file path: ", p))
}

// eitherPath returns the directory and/or file path interpretation of p. If dir
// is empty, then p is invalid. Otherwise, file may be empty if p was "." or had
// a trailing '/'.
func eitherPath(p string) (dir, file path) {
	if c := path(cleanPath(p)); c.isDir() {
		dir = c
	} else if c != "" {
		c += "/"
		dir, file = c, c[:len(p)-1]
	}
	return
}

// String returns the path as a string.
func (p path) String() string { return string(p) }

// contains returns whether other is under the directory tree p. It returns true
// if the paths are equal (same directory) or if p is ".".
func (p path) contains(other path) bool {
	return p == "." || (0 < len(p) && len(p) <= len(other) &&
		other[:len(p)] == p && p[len(p)-1] == '/')
}

// dir returns the parent directory of p. It panics if p is empty.
func (p path) dir() path {
	if p == "" {
		panic(emptyPath)
	}
	if i := strings.LastIndexByte(string(p[:len(p)-1]), '/'); i > 0 {
		return p[:i+1]
	}
	return "."
}

// base returns the last element of p. It panics if p is empty.
func (p path) base() string {
	if p == "" {
		panic(emptyPath)
	}
	if p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	if i := strings.LastIndexByte(string(p), '/'); i > 0 {
		return string(p[i+1:])
	}
	return string(p)
}

// commonRoot returns the directory path that is a parent of both p and other.
func (p path) commonRoot(other path) path {
	a, b := string(p), string(other)
	for {
		i := strings.IndexByte(a, '/')
		if i < 0 || len(b) < i+1 || a[:i+1] != b[:i+1] {
			break
		}
		a, b = a[i+1:], b[i+1:]
	}
	if r := p[:len(p)-len(a)]; r != "" {
		return r
	}
	if p != "" && other != "" {
		return "."
	}
	panic(emptyPath)
}

// dist returns the distance between two paths in terms of directories traversed
// to go from one to the other.
func (p path) dist(other path) int {
	if r := p.commonRoot(other); r != "." {
		p, other = p[len(r):], other[len(r):]
	}
	return strings.Count(string(p), "/") + strings.Count(string(other), "/")
}

// isDir returns whether the path refers to a directory.
func (p path) isDir() bool {
	return 0 < len(p) && (p == "." || p[len(p)-1] == '/')
}

// isFile returns whether the path refers to a file.
func (p path) isFile() bool {
	return 0 < len(p) && p != "." && p[len(p)-1] != '/'
}

// cmp returns -1 if p < other, 0 if p == other, and +1 if p > other.
// Directories are less than files. It panics if either path is empty or if the
// same name refers to both a file and a directory.
func (p path) cmp(other path) int {
	lessIf := func(b bool) int {
		if b {
			return -1
		}
		return +1
	}
	a, b := string(p), string(other)

	// Handle empty paths and root, which is less than all non-root paths
	if len(a) <= 1 || len(b) <= 1 {
		if a == "" || b == "" {
			panic(emptyPath)
		}
		if a == "." || b == "." {
			if a == b {
				return 0
			}
			return lessIf(a == ".")
		}
	}

	// Find the first byte mismatch
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			// Path separator is less than any other byte
			if aSep := a[i] == '/'; aSep || b[i] == '/' {
				return lessIf(aSep)
			}
			// Directory is less than a file
			aDir := strings.IndexByte(a[i+1:], '/') >= 0
			if aDir != (strings.IndexByte(b[i+1:], '/') >= 0) {
				return lessIf(aDir)
			}
			return lessIf(a[i] < b[i])
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

	// If a ends with '/', then it's a parent of b. If b does not have any more
	// separators, then a and b are regular files in the same directory and a is
	// shorter. Otherwise, a is a file and b is a directory.
	return lessIf((a[len(a)-1] == '/' || bSep < 0) != invert)
}

// steps iterates over every step in a path.
type steps struct {
	p path
	n int
}

// next returns the next step in s or "" if the last step was reached. It does
// not return the root directory.
func (s *steps) next() path {
	if s.n >= len(s.p) || s.p == "." {
		return ""
	}
	if i := strings.IndexByte(string(s.p[s.n:]), '/'); i > 0 {
		s.n += i + 1
	} else if s.n = len(s.p); i == 0 {
		panic(fmt.Sprint("index: rooted or non-clean path: ", s)) // Shouldn't happen
	}
	return s.p[:s.n]
}

// skip causes next to return the step after p if p is a parent of the final
// path (p.contains(s.path) == true) and hasn't been returned yet.
func (s *steps) skip(p path) {
	if s.n < len(p) && len(p) <= len(s.p) &&
		s.p[:len(p)] == p && p[len(p)-1] == '/' {
		s.n = len(p)
	}
}

// uniqueDirs visits all unique directories in a set of paths.
type uniqueDirs []steps

// add adds path p to the set. It panics if p is not a directory.
func (u *uniqueDirs) add(p path) {
	if !p.isDir() {
		panic(fmt.Sprint("index: not a directory: ", p))
	}
	*u = append(*u, steps{p: p})
}

// clear removes all paths from the set.
func (u *uniqueDirs) clear() { *u = (*u)[:0] }

// forEach calls fn for each unique directory in the set, leaving the set empty.
// For example, if the set contains paths "A/B/", "A/C/", and "D/", fn will be
// called for ".", "A/", "A/B/", "A/C/", and "D/".
func (u *uniqueDirs) forEach(fn func(path)) {
	if len(*u) == 0 {
		return
	}
	defer u.clear()
	fn(".")
	for i := 0; i < len(*u); {
		if p := (*u)[i].next(); p != "" {
			fn(p)
			for j := i + 1; j < len(*u); j++ {
				(*u)[j].skip(p)
			}
		} else {
			i++
		}
	}
}

// cleanPath returns a clean, slash-separated representation of p. It returns ""
// if p is invalid. If p is a directory, the returned path will be either "." or
// end with a '/'.
func cleanPath(p string) string {
	// VolumeName != "" is a superset of filepath.IsAbs test on Windows
	if p == "" || filepath.VolumeName(p) != "" {
		return ""
	}

	// path.Clean is more efficient than filepath.Clean and we want to return ""
	// for cases handled by filepath.postClean.
	p = filepath.ToSlash(p)
	c := stdpath.Clean(p)
	if (len(c) > 3 && c[:3] == "../") ||
		(len(c) > 1 && (c[1] == ':' || c == "..")) ||
		len(c) == 0 || c[0] == '/' {
		return ""
	}

	// Preserve directory status
	switch p[len(p)-1] {
	case '/':
		if c == "." {
			return "."
		}
		if len(p) == len(c)+1 && p[:len(c)] == c {
			c = p // Avoid allocation
		} else {
			c += "/"
		}
	case '.':
		if c == "." {
			return "."
		}
		if (len(p) > 3 && p[len(p)-3:] == "/..") ||
			(len(p) > 1 && p[len(p)-2] == '/') {
			c += "/"
		}
	}
	return c
}
