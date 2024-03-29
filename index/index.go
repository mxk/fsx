package index

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/klauspost/compress/zstd"
	"github.com/rivo/uniseg"
)

// Index is the root of an indexed file system.
type Index struct {
	root   string
	groups []Files
}

// New creates a new file index.
func New(root string, all Files) *Index {
	if len(all) == 0 {
		return &Index{root: root}
	}
	return &Index{root, groupByDigest(all)}
}

// Load loads index contents from the specified file path.
func Load(name string) (*Index, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()
	x, err := Read(f)
	err2 := f.Close()
	if f = nil; err == nil {
		err = err2
	}
	return x, err
}

// Save saves index contents to the specified file path. If the file already
// exists, it is first renamed with a ".bak" extension.
func (x *Index) Save(name string) error { return x.save(name, true) }

// Overwrite saves index contents to the specified file path. If the file
// already exists, it is overwritten.
func (x *Index) Overwrite(name string) error { return x.save(name, false) }

// save saves index contents to the specified file path. If backup is true and
// the file already exists, it is first renamed by adding a ".bak" extension.
func (x *Index) save(name string, backup bool) (err error) {
	name = filepath.Clean(name)
	f, err := os.CreateTemp(filepath.Dir(name), filepath.Base(name)+".*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
		}
	}()
	if err = x.Write(f); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if backup {
		if fi, err := os.Lstat(name); err == nil && !fi.Mode().IsRegular() {
			return fmt.Errorf("index: cannot backup irregular file: %s", name)
		}
		err = os.Rename(name, name+".bak")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return os.Rename(f.Name(), name)
}

// Read reads index contents from src.
func Read(src io.Reader) (*Index, error) {
	r, err := zstd.NewReader(src)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return read(r)
}

// read reads uncompressed index contents from src.
func read(src io.Reader) (*Index, error) {
	s := bufio.NewScanner(src)
	line, root, err := readHeader(s)
	if err != nil {
		return nil, err
	}
	var g Files
	groups := make([]Files, 0, 512)
	for ; s.Scan(); line++ {
		ln, ok := bytes.CutPrefix(s.Bytes(), []byte("\t\t"))
		if !ok {
			// Flags
			flagb, ln, ok := cutByte(ln, '\t')
			if !ok {
				return nil, fmt.Errorf("index: invalid entry on line %d", line)
			}
			flag, ok := parseFlag(flagb)
			if !ok {
				return nil, fmt.Errorf("index: invalid flag on line %d (%s)", line, flagb)
			}

			// Path and modification time
			p, ln, _ := bytes.Cut(ln, []byte("\t//"))
			f := &File{path: strictFilePath(string(p)), flag: flag}
			if len(ln) > 0 {
				if err := f.modTime.UnmarshalText(bytes.TrimLeft(ln, "\t")); err != nil {
					return nil, fmt.Errorf("index: invalid modification time on line %d", line)
				}
			} else if len(g) == 0 {
				return nil, fmt.Errorf("index: missing modification time on line %d", line)
			} else {
				f.modTime = g[len(g)-1].modTime
			}

			if len(g) == cap(g) && len(g) < 256 {
				g = append(make(Files, 0, 512), g...)
			}
			g = append(g, f)
			continue
		}
		if len(g) == 0 {
			return nil, fmt.Errorf("index: missing file group before line %d", line)
		}

		// Digest
		digest, ln, ok := cutByte(ln, '\t')
		n, err := hex.Decode(g[0].digest[:], digest)
		if !ok || n != len(Digest{}) || err != nil {
			return nil, fmt.Errorf("index: invalid digest on line %d", line)
		}

		// Size
		v, err := strconv.ParseUint(unsafeString(ln), 10, 63)
		if g[0].size = int64(v); err != nil {
			return nil, fmt.Errorf("index: invalid size on line %d", line)
		}

		// Copy digest and size
		for _, f := range g[1:] {
			f.digest, f.size = g[0].digest, g[0].size
		}
		groups, g = append(groups, g[:len(g):len(g)]), g[len(g):]
	}
	if s.Err() != nil {
		return nil, fmt.Errorf("index: read error on line %d (%w)", line, s.Err())
	}
	if len(g) != 0 {
		return nil, fmt.Errorf("index: incomplete final group")
	}
	return &Index{root, groups}, nil
}

const v1 = "fsx index v1"

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
func (x *Index) Write(dst io.Writer) error {
	w, err := zstd.NewWriter(dst)
	if err != nil {
		panic(err) // Invalid option(s)
	}
	if err = x.write(w); err == nil {
		err = w.Close()
	}
	return err
}

// write writes uncompressed index contents to dst.
func (x *Index) write(dst io.Writer) error {
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
	x.writeHeader(w)
	lineWidth := make([]int, 0, 16)
	for _, g := range x.groups {
		// Calculate path widths
		empty := true
		align, lineWidth := minAlign, lineWidth[:0]
		for _, f := range g {
			if f.flag.write() {
				empty = false
				n := tabWidth + width(string(f.path))&^(tabWidth-1) + 2*tabWidth
				align, lineWidth = max(align, n), append(lineWidth, n)
			} else {
				lineWidth = append(lineWidth, 0)
			}
			if f.digest != g[0].digest || f.size != g[0].size {
				panic(fmt.Sprint("index: group digest/size mismatch: ", f))
			}
		}
		if empty {
			continue
		}

		// Flags, paths, and modification times
		for i, f := range g {
			if !f.flag.write() {
				continue
			}
			_, _ = w.WriteString(f.flag.String())
			_ = w.WriteByte('\t')
			_, _ = w.WriteString(string(f.path))
			if i == 0 || !f.modTime.Equal(g[i-1].modTime) {
				_, _ = w.WriteString("\t//\t")
				n := (align - lineWidth[i]) / tabWidth
				b := buf(w, n+len(time.RFC3339Nano))[:n]
				for i := range b {
					b[i] = '\t'
				}
				_, _ = w.Write(f.modTime.AppendFormat(b, time.RFC3339Nano))
			} else if p := string(f.path); strings.TrimRight(p, "\t\n\v\f\r ") != p {
				_, _ = w.WriteString("\t//")
			}
			_ = w.WriteByte('\n')
		}

		// Digest
		_, _ = w.WriteString("\t\t")
		b := buf(w, digestHex)[:digestHex]
		hex.Encode(b, g[0].digest[:])
		_, _ = w.Write(b)

		// Size
		b = append(buf(w, len("\t18446744073709551615")), '\t')
		_, _ = w.Write(strconv.AppendUint(b, uint64(g[0].size), 10))
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

// writeHeader writes the index version and root path to w.
func (x *Index) writeHeader(w *bufio.Writer) {
	_, _ = w.WriteString(v1)
	_ = w.WriteByte('\n')
	_, _ = w.WriteString(x.root)
	_ = w.WriteByte('\n')
}

// Root returns the index root directory.
func (x *Index) Root() string { return x.root }

// Files returns all files.
func (x *Index) Files() Files {
	var n int
	for _, g := range x.groups {
		n += len(g)
	}
	all := make(Files, 0, n)
	for _, g := range x.groups {
		all = append(all, g...)
	}
	all.Sort()
	return all
}

// groupByDigest combines files with identical digests into groups. The relative
// file order within each group is preserved.
func groupByDigest(all Files) []Files {
	type group struct {
		i int
		f Files
	}
	idx := make(map[Digest]group, len(all))
	for i, f := range all {
		g, ok := idx[f.digest]
		if !ok {
			g.i = i
		} else if g.f[0].size != f.size {
			panic(fmt.Sprintf("index: digest collision: %q != %q", g.f[0].path, f.path))
		}
		g.f = append(g.f, f)
		idx[f.digest] = g
	}
	groups := make([]Files, 0, len(idx))
	for _, g := range idx {
		groups = append(groups, g.f)
	}
	slices.SortFunc(groups, func(a, b Files) int {
		return cmp.Compare(idx[a[0].digest].i, idx[b[0].digest].i)
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

// width returns the rendered monospace width of s with proper tab alignment.
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
