package index

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/zeebo/blake3"
)

// NewHash creates new hash state.
var NewHash = blake3.New

func init() {
	if len(Digest{}) != NewHash().Size() {
		panic("hash size mismatch")
	}
}

// Index is the root of an indexed file system.
type Index struct {
	Root   string
	Groups []Files
}

// Load loads index contents from the specified file path.
func Load(path string) Index {
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	r := Read(f)
	if err = f.Close(); err != nil {
		panic(err)
	}
	return r
}

// Read reads index contents from src.
func Read(src io.Reader) Index {
	s := bufio.NewScanner(src)
	if !s.Scan() || len(s.Bytes()) == 0 || bytes.Contains(s.Bytes(), []byte{'\t'}) {
		panic("invalid root")
	}
	idx := Index{Root: s.Text(), Groups: make([]Files, 0, 128)}
	var g Files
	for line := 2; s.Scan(); line++ {
		if ln, ok := bytes.CutPrefix(s.Bytes(), []byte("\t\t")); ok {
			if len(g) == 0 {
				panic(fmt.Sprint("missing file group before line ", line))
			}
			digest, size, ok := bytes.Cut(ln, []byte{'\t'})
			if n, err := hex.Decode(g[0].Digest[:], digest); n != len(Digest{}) || err != nil {
				panic(fmt.Sprint("invalid digest on line ", line))
			}
			v, err := strconv.ParseUint(string(size), 10, 63)
			if !ok || err != nil {
				panic(fmt.Sprint("invalid size on line ", line))
			}
			g[0].Size = int64(v)
			for _, f := range g[1:] {
				f.Size, f.Digest = g[0].Size, g[0].Digest
			}
			idx.Groups = append(idx.Groups, g)
			g = nil
		} else {
			attr, path, ok := bytes.Cut(s.Bytes(), []byte{'\t'})
			if !ok || len(attr) > 3 || len(path) == 0 {
				panic(fmt.Sprint("invalid entry on line ", line))
			}
			g = append(g, &File{Path: string(path)})
		}
	}
	if err := s.Err(); err != nil {
		panic(err)
	}
	if len(g) != 0 {
		panic("incomplete final file group")
	}
	return idx
}

// Write writes index contents to dst.
func (idx Index) Write(dst io.Writer) error {
	var digest [2*len(Digest{})]byte
	w := bufio.NewWriter(dst)
	_, _ = w.WriteString(filepath.ToSlash(idx.Root))
	_ = w.WriteByte('\n')
	for _, g := range idx.Groups {
		for _, f := range g {
			_ = w.WriteByte('\t')
			_, _ = w.WriteString(f.Path)
			_ = w.WriteByte('\n')
		}
		_, _ = w.WriteString("\t\t")
		hex.Encode(digest[:], g[0].Digest[:])
		_, _ = w.Write(digest[:])
		_ = w.WriteByte('\t')
		_, _ = fmt.Fprintln(w, g[0].Size)
	}
	return w.Flush()
}

// File is a regular file in the file system.
type File struct {
	Path   string
	Size   int64
	Digest Digest
}

// Digest is the output of the hash function.
type Digest [32]byte

// Files is an ordered list of files.
type Files []*File

// Sort sorts files by path.
func (all Files) Sort() {
	sort.SliceStable(all, func(i, j int) bool {
		return pathLess(all[i].Path, all[j].Path)
	})
}

// pathLess returns whether path a should be sorted before path b.
func pathLess(a, b string) bool {
	isDir := func(s string) bool {
		return strings.IndexByte(s, '/') >= 0
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if ca, cb := a[i], b[i]; ca != cb {
			if aDir := isDir(a[i+1:]); aDir != isDir(b[i+1:]) {
				return aDir
			}
			if ca == '/' {
				ca = 0
			} else if cb == '/' {
				cb = 0
			}
			return ca < cb
		}
	}
	return len(a) < len(b) && !isDir(b[len(a):])
}
