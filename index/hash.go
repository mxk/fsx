package index

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/zeebo/blake3"
)

// Digest is the output of the hash function.
type Digest [32]byte

// newHash creates new hash state.
var newHash = blake3.New

func init() {
	if len(Digest{}) != newHash().Size() {
		panic("index: hash size mismatch")
	}
}

// Hasher is a file hasher.
type Hasher struct {
	h blake3.Hasher
	b [1024 * 1024]byte
}

// NewHasher returns a new file hasher.
func NewHasher() *Hasher { return &Hasher{h: *newHash()} }

// Read computes the digest of the specified file. If fsys is nil, it will be
// set to the parent directory of name.
func (h *Hasher) Read(fsys fs.FS, name string) (*File, error) {
	if fsys == nil {
		fsys, name = os.DirFS(filepath.Dir(name)), filepath.Base(name)
	}
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("index: failed to open file: %s (%w)", name, err)
	}
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("index: failed to stat file: %s (%w)", name, err)
	}

	// Compute digest
	h.h.Reset()
	n, err := io.CopyBuffer(&h.h, f, h.b[:])
	f, err2 := nil, f.Close()
	if err != nil {
		return nil, fmt.Errorf("index: failed to read file: %s (%w)", name, err)
	}
	if err2 != nil {
		return nil, fmt.Errorf("index: failed to close file: %s (%w)", name, err2)
	}
	if n != fi.Size() {
		return nil, fmt.Errorf("index: file size mistmach: %s (want %d, got %d)", name, fi.Size(), n)
	}
	if n == 0 {
		// Zero-length files get a unique hash based on their full name
		_, _ = h.h.WriteString(name)
	}

	// Verify that file size and modtime have not changed
	fi2, err := fs.Stat(fsys, name)
	if err != nil || fi.Size() != fi2.Size() || fi.ModTime() != fi2.ModTime() {
		return nil, fmt.Errorf("index: file modified while reading: %s", name)
	}

	file := &File{filePath(name), h.digest(), fi.Size(), fi.ModTime(), flagNone}
	return file, nil
}

// digest returns the current hash Digest.
func (h *Hasher) digest() (d Digest) {
	if b := h.h.Sum(d[:0]); &b[len(b)-1] != &d[len(d)-1] {
		panic("index: digest buffer reallocated")
	}
	return
}
