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
	"time"
)

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
			g = append(g, &File{Path: Path{string(path)}})
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
	const DigestHex = 2 * len(Digest{})
	var buf [max(DigestHex, len(time.RFC3339Nano))]byte
	w := bufio.NewWriter(dst)
	_, _ = w.WriteString(filepath.ToSlash(idx.Root))
	_ = w.WriteByte('\n')
	for _, g := range idx.Groups {
		var mod time.Time
		for _, f := range g {
			_ = w.WriteByte('\t')
			_, _ = w.WriteString(f.p)
			_ = w.WriteByte('\n')
			if mod.Before(f.Mod) {
				mod = f.Mod
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
func (all Files) Sort() {
	sort.SliceStable(all, func(i, j int) bool {
		return all[i].less(all[j].Path)
	})
}
