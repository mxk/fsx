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
)

// Index is the root of an indexed file system.
type Index struct {
	Root   string
	Groups []Files
}

// File is a regular file in the file system.
type File struct {
	Path
	Digest Digest
	Size   int64
	Mod    time.Time
}

// Files is an ordered list of files.
type Files []*File

// Sort sorts files by path.
func (fs Files) Sort() {
	sort.Slice(fs, func(i, j int) bool { return fs[i].less(fs[j].Path) })
}

// Load loads index contents from the specified file path.
func Load(name string) (Index, error) {
	f, err := os.Open(name)
	if err != nil {
		return Index{}, err
	}
	idx, err := Read(f)
	if err2 := f.Close(); err == nil && err2 != nil {
		err = err2
	}
	return idx, err
}

// Read reads index contents from src.
func Read(src io.Reader) (Index, error) {
	s := bufio.NewScanner(src)
	line, root, err := readHeader(s)
	if err != nil {
		return Index{}, err
	}
	var g Files
	groups := make([]Files, 0, 512)
	for ; s.Scan(); line++ {
		if ln, ok := bytes.CutPrefix(s.Bytes(), []byte("\t\t")); ok {
			if len(g) == 0 {
				return Index{}, fmt.Errorf("index: missing file group before line %d", line)
			}
			digest, tail, ok := bytes.Cut(ln, []byte{'\t'})
			if n, err := hex.Decode(g[0].Digest[:], digest); !ok || n != len(Digest{}) || err != nil {
				return Index{}, fmt.Errorf("index: invalid digest on line %d", line)
			}
			size, mod, ok := bytes.Cut(tail, []byte{'\t'})
			v, err := strconv.ParseUint(string(size), 10, 63)
			if g[0].Size = int64(v); !ok || err != nil {
				return Index{}, fmt.Errorf("index: invalid size on line %d", line)
			}
			if g[0].Mod, err = time.Parse(time.RFC3339Nano, string(mod)); err != nil {
				return Index{}, fmt.Errorf("index: invalid modification time on line %d", line)
			}
			for _, f := range g[1:] {
				f.Digest, f.Size, f.Mod = g[0].Digest, g[0].Size, g[0].Mod
			}
			groups = append(groups, append(make(Files, 0, len(g)), g...))
			g = g[:0]
		} else {
			attr, p, ok := bytes.Cut(s.Bytes(), []byte{'\t'})
			if !ok || len(attr) > 3 || len(p) == 0 {
				return Index{}, fmt.Errorf("index: invalid entry on line %d", line)
			}
			g = append(g, &File{Path: filePath(string(p))})
		}
	}
	if s.Err() != nil {
		return Index{}, fmt.Errorf("index: read error: %w", s.Err())
	}
	if len(g) != 0 {
		return Index{}, fmt.Errorf("index: incomplete final group")
	}
	return Index{root, groups}, nil
}

// Write writes index contents to dst.
func (idx Index) Write(dst io.Writer) error {
	const DigestHex = 2 * len(Digest{})
	var buf [max(DigestHex, len(time.RFC3339Nano))]byte
	now := time.Now()
	w := bufio.NewWriter(dst)
	writeHeader(w, idx.Root)
	for _, g := range idx.Groups {
		var mod time.Time
		for _, f := range g {
			_ = w.WriteByte('\t')
			_, _ = w.WriteString(f.p)
			_ = w.WriteByte('\n')
			if f.Digest != g[0].Digest || f.Size != g[0].Size {
				panic(fmt.Sprintf("index: group mismatch: %s", f))
			}
			if mod.Before(f.Mod) {
				if mod = f.Mod; !mod.Before(now) {
					panic(fmt.Sprintf("index: modification time in the future: %s", f))
				}
			}
		}
		_, _ = w.WriteString("\t\t")
		hex.Encode(buf[:DigestHex], g[0].Digest[:])
		_, _ = w.Write(buf[:DigestHex])
		_ = w.WriteByte('\t')
		_, _ = w.Write(strconv.AppendUint(buf[:0], uint64(g[0].Size), 10))
		_ = w.WriteByte('\t')
		_, _ = w.Write(mod.AppendFormat(buf[:0], time.RFC3339Nano))
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

const hdr = "fsx index v1"

func readHeader(s *bufio.Scanner) (line int, root string, err error) {
	if line++; !s.Scan() {
		err = fmt.Errorf("index: missing signature: %w", s.Err())
		return
	}
	if sig := s.Bytes(); string(sig) != hdr {
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

func writeHeader(w *bufio.Writer, root string) {
	_, _ = w.WriteString(hdr)
	_ = w.WriteByte('\n')
	_, _ = w.WriteString(root)
	_ = w.WriteByte('\n')
}
