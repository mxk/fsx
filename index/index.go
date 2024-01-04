package index

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

const v1 = "fsx index v1"

// Index is the root of an indexed file system.
type Index struct {
	root   string
	groups []Files
}

// File is a regular file in the file system.
type File struct {
	Path
	Digest  Digest
	Size    int64
	ModTime time.Time
}

// Files is an ordered list of files.
type Files []*File

// Sort sorts files by path.
func (fs Files) Sort() {
	sort.Slice(fs, func(i, j int) bool { return fs[i].less(fs[j].Path) })
}

// New creates a new file index.
func New(root string, all Files) Index {
	return Index{root, groupByDigest(all)}
}

// Load loads index contents from the specified file path.
func Load(name string) (Index, error) {
	f, err := os.Open(name)
	if err != nil {
		return Index{}, err
	}
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()
	idx, err := Read(f)
	f, err2 := nil, f.Close()
	if err == nil && err2 != nil {
		err = err2
	}
	return idx, err
}

// Read reads index contents from src.
func Read(src io.Reader) (Index, error) {
	r, err := zstd.NewReader(src)
	if err != nil {
		return Index{}, err
	}
	defer r.Close()
	return read(r)
}

const timeFmt = time.RFC3339Nano

// read reads uncompressed index contents from src.
func read(src io.Reader) (Index, error) {
	s := bufio.NewScanner(src)
	line, root, err := readHeader(s)
	if err != nil {
		return Index{}, err
	}
	var g Files
	groups := make([]Files, 0, 512)
	for ; s.Scan(); line++ {
		ln, ok := bytes.CutPrefix(s.Bytes(), []byte("\t\t"))
		if !ok {
			// File path
			attr, p, ok := cutByte(ln, '\t')
			if !ok || len(attr) > 3 || len(p) == 0 {
				return Index{}, fmt.Errorf("index: invalid entry on line %d", line)
			}
			if len(g) == cap(g) && len(g) < 256 {
				g = append(make(Files, 0, 512), g...)
			}
			g = append(g, &File{Path: filePath(string(p))})
			continue
		}
		if len(g) == 0 {
			return Index{}, fmt.Errorf("index: missing file group before line %d", line)
		}

		// Digest
		digest, ln, ok := cutByte(ln, '\t')
		if n, err := hex.Decode(g[0].Digest[:], digest); !ok || n != len(Digest{}) || err != nil {
			return Index{}, fmt.Errorf("index: invalid digest on line %d", line)
		}

		// Size
		size, ln, ok := cutByte(ln, '\t')
		v, err := strconv.ParseUint(string(size), 10, 63)
		if g[0].Size = int64(v); !ok || err != nil {
			return Index{}, fmt.Errorf("index: invalid size on line %d", line)
		}

		// Modification time(s)
		var mod []byte
		for i := 0; len(ln) > 0; i++ {
			if i >= len(g) {
				return Index{}, fmt.Errorf("index: extra modification time(s) on line %d", line)
			}
			if mod, ln, _ = cutByte(ln, ','); len(mod) != 0 {
				if g[i].ModTime, err = time.Parse(timeFmt, string(mod)); err != nil {
					return Index{}, fmt.Errorf("index: invalid modification time on line %d", line)
				}
			}
		}
		if g[0].ModTime.IsZero() {
			return Index{}, fmt.Errorf("index: missing modification time on line %d", line)
		}

		// Copy digest, size, and modification times
		for prev, f := range g[1:] {
			if f.Digest, f.Size = g[prev].Digest, g[prev].Size; f.ModTime.IsZero() {
				f.ModTime = g[prev].ModTime
			}
		}
		groups, g = append(groups, g[:len(g):len(g)]), g[len(g):]
	}
	if s.Err() != nil {
		return Index{}, fmt.Errorf("index: read error on line %d (%w)", line, s.Err())
	}
	if len(g) != 0 {
		return Index{}, fmt.Errorf("index: incomplete final group")
	}
	return Index{root, groups}, nil
}

// readHeader reads the index version and root path lines from s.
func readHeader(s *bufio.Scanner) (line int, root string, err error) {
	if line++; !s.Scan() {
		err = fmt.Errorf("index: missing signature: %w", s.Err())
		return
	}
	if sig := s.Bytes(); string(sig) != v1 {
		err = fmt.Errorf("index: invalid signature: %s", sig)
		return
	}
	if line++; !s.Scan() {
		err = fmt.Errorf("index: missing root: %w", s.Err())
		return
	}
	p := s.Text()
	if p != path.Clean(filepath.ToSlash(p)) || strings.IndexByte(p, '\t') >= 0 {
		err = fmt.Errorf("index: invalid root: %s", p)
		return
	}
	return line + 1, p, nil
}

// Write writes index contents to dst.
func (idx Index) Write(dst io.Writer) error {
	w, err := zstd.NewWriter(dst)
	if err != nil {
		return err
	}
	if err = idx.WriteRaw(w); err == nil {
		err = w.Close()
	}
	return err
}

// WriteRaw writes uncompressed index contents to dst.
func (idx Index) WriteRaw(dst io.Writer) error {
	const digestHex = 2 * len(Digest{})
	var tmpBuf [max(digestHex, len(timeFmt))]byte
	w := bufio.NewWriter(dst)
	buf := func() []byte {
		b := w.AvailableBuffer()
		if cap(b) < len(tmpBuf) {
			b = tmpBuf[:0]
		}
		return b
	}
	writeHeader(w, idx.root)
	for _, g := range idx.groups {
		// File paths
		for _, f := range g {
			_ = w.WriteByte('\t')
			_, _ = w.WriteString(f.p)
			_ = w.WriteByte('\n')
			if f.Digest != g[0].Digest || f.Size != g[0].Size {
				panic(fmt.Sprintf("index: group mismatch: %s", f))
			}
		}

		// Digest
		_, _ = w.WriteString("\t\t")
		b := buf()[:digestHex]
		hex.Encode(b, g[0].Digest[:])
		_, _ = w.Write(b)

		// Size
		_ = w.WriteByte('\t')
		_, _ = w.Write(strconv.AppendUint(buf(), uint64(g[0].Size), 10))
		_ = w.WriteByte('\t')

		// Modification time(s)
		_, _ = w.Write(g[0].ModTime.AppendFormat(buf(), timeFmt))
		prev := 0
		for i, f := range g[1:] {
			if !f.ModTime.Equal(g[prev].ModTime) {
				for ; prev <= i; prev++ {
					_ = w.WriteByte(',')
				}
				_, _ = w.Write(f.ModTime.AppendFormat(buf(), timeFmt))
			}
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

// writeHeader writes the index version and root path to w.
func writeHeader(w *bufio.Writer, root string) {
	_, _ = w.WriteString(v1)
	_ = w.WriteByte('\n')
	_, _ = w.WriteString(root)
	_ = w.WriteByte('\n')
}

// groupByDigest combines files with identical digests into groups. The returned
// slice is sorted by the first file in each group.
func groupByDigest(all Files) []Files {
	type group struct {
		i int
		f Files
	}
	all.Sort()
	idx := make(map[Digest]group, len(all))
	for i, f := range all {
		g, ok := idx[f.Digest]
		if !ok {
			g.i = i
		} else if g.f[0].Size != f.Size {
			panic(fmt.Sprintf("index: hash collision: %q != %q", g.f[0].Path, f.Path))
		}
		g.f = append(g.f, f)
		idx[f.Digest] = g
	}
	groups := make([]Files, 0, len(idx))
	for _, g := range idx {
		groups = append(groups, g.f)
	}
	sort.Slice(groups, func(i, j int) bool {
		return idx[groups[i][0].Digest].i < idx[groups[j][0].Digest].i
	})
	return groups
}

// cutByte is bytes.Cut for a one-byte separator.
func cutByte(s []byte, sep byte) (before, after []byte, found bool) {
	if i := bytes.IndexByte(s, sep); i >= 0 {
		return s[:i], s[i+1:], true
	}
	return s, nil, false
}
