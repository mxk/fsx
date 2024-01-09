package index

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/klauspost/compress/zstd"
	"github.com/rivo/uniseg"
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
			// Attributes
			attr, ln, ok := cutByte(ln, '\t')
			if !ok || len(attr) > 3 {
				return Index{}, fmt.Errorf("index: invalid entry on line %d", line)
			}

			// Path and modification time
			p, mod, ok := bytes.Cut(ln, []byte("\t//\t"))
			f := &File{Path: filePath(string(p))}
			if ok {
				if err := f.ModTime.UnmarshalText(bytes.TrimLeft(mod, "\t")); err != nil {
					return Index{}, fmt.Errorf("index: invalid modification time on line %d", line)
				}
			} else if len(g) == 0 {
				return Index{}, fmt.Errorf("index: missing modification time on line %d", line)
			} else {
				f.ModTime = g[len(g)-1].ModTime
			}
			if len(g) == cap(g) && len(g) < 256 {
				g = append(make(Files, 0, 512), g...)
			}
			g = append(g, f)
			continue
		}
		if len(g) == 0 {
			return Index{}, fmt.Errorf("index: missing file group before line %d", line)
		}

		// Digest
		digest, size, ok := cutByte(ln, '\t')
		n, err := hex.Decode(g[0].Digest[:], digest)
		if !ok || n != len(Digest{}) || err != nil {
			return Index{}, fmt.Errorf("index: invalid digest on line %d", line)
		}

		// Size
		v, err := strconv.ParseUint(unsafeString(size), 10, 63)
		if g[0].Size = int64(v); !ok || err != nil {
			return Index{}, fmt.Errorf("index: invalid size on line %d", line)
		}

		// Copy digest and size
		for _, f := range g[1:] {
			f.Digest, f.Size = g[0].Digest, g[0].Size
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
	if sig := unsafeString(s.Bytes()); sig != v1 {
		err = fmt.Errorf("index: invalid signature: %s", sig)
		return
	}
	if line++; !s.Scan() {
		err = fmt.Errorf("index: missing root: %w", s.Err())
		return
	}
	if root = s.Text(); strings.IndexByte(root, '\t') >= 0 {
		err = fmt.Errorf("index: invalid root: %s", root)
		return
	}
	return line + 1, root, nil
}

// Write writes index contents to dst.
func (idx *Index) Write(dst io.Writer) error {
	w, err := zstd.NewWriter(dst)
	if err != nil {
		return err
	}
	if err = idx.write(w); err == nil {
		err = w.Close()
	}
	return err
}

// write writes uncompressed index contents to dst.
func (idx *Index) write(dst io.Writer) error {
	const digestHex = 2 * len(Digest{})
	const minAlign = 2*tabWidth + digestHex + tabWidth + (11 &^ (tabWidth - 1)) + tabWidth
	buf := func(w *bufio.Writer, c int) (b []byte) {
		if b = w.AvailableBuffer(); cap(b) < c {
			err := w.Flush()
			if b = w.AvailableBuffer(); cap(b) < c {
				if err == nil {
					panic(fmt.Sprintf("index: insufficient write buffer (have %d, want %d)", cap(b), c))
				}
				b = make([]byte, 0, c)
			}
		}
		return
	}
	w := bufio.NewWriter(dst)
	idx.writeHeader(w)
	lineWidth := make([]int, 0, 16)
	for _, g := range idx.groups {
		// Calculate file path widths
		align, lineWidth := minAlign, lineWidth[:0]
		for i, f := range g {
			n := width(f.p)&^(tabWidth-1) + 3*tabWidth
			align, lineWidth = max(align, n), append(lineWidth, n)
			if i != 0 && (f.Digest != g[0].Digest || f.Size != g[0].Size) {
				panic(fmt.Sprintf("index: group mismatch: %s", f))
			}
		}

		// Paths and modification times
		var t time.Time
		for i, f := range g {
			_ = w.WriteByte('\t')
			_, _ = w.WriteString(f.p)
			if !f.ModTime.Equal(t) {
				t = f.ModTime
				_, _ = w.WriteString("\t//\t")
				n := (align - lineWidth[i]) / tabWidth
				b := buf(w, n+len(timeFmt))[:n]
				for i := range b {
					b[i] = '\t'
				}
				_, _ = w.Write(f.ModTime.AppendFormat(b, timeFmt))
			}
			_ = w.WriteByte('\n')
		}

		// Digest
		_, _ = w.WriteString("\t\t")
		b := buf(w, digestHex+1)[:digestHex]
		hex.Encode(b, g[0].Digest[:])
		_, _ = w.Write(append(b, '\t'))

		// Size
		b = buf(w, len("18446744073709551615"))
		_, _ = w.Write(strconv.AppendUint(b, uint64(g[0].Size), 10))
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

// writeHeader writes the index version and root path to w.
func (idx *Index) writeHeader(w *bufio.Writer) {
	_, _ = w.WriteString(v1)
	_ = w.WriteByte('\n')
	_, _ = w.WriteString(idx.root)
	_ = w.WriteByte('\n')
}

// Root returns the index root directory.
func (idx *Index) Root() string {
	return idx.root
}

// All returns all files.
func (idx *Index) All() Files {
	all := make(Files, 0, len(idx.groups))
	for _, g := range idx.groups {
		all = append(all, g...)
	}
	all.Sort()
	return all
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

// unsafeString converts a byte slice to a string without copying.
func unsafeString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(&b[0], len(b))
}

// tabWidth is a power-of-2 tab character alignment.
const tabWidth = 1 << 3

// width returns the rendered monospace width of s with tab alignment.
func width(s string) (n int) {
	for {
		i := strings.IndexByte(s, '\t')
		if i < 0 {
			return n + uniseg.StringWidth(s)
		}
		j := i + 1
		for j < len(s) && s[j] == '\t' {
			j++
		}
		n = (n+uniseg.StringWidth(s[:i]))&^(tabWidth-1) + (j-i)*tabWidth
		s = s[j:]
	}
}
