package index

import (
	"fmt"
	"io"
	"io/fs"

	"github.com/zeebo/blake3"
)

// Digest is the output of the hash function.
type Digest [32]byte

// newHash creates new hash state.
var newHash = blake3.New

func init() {
	if len(Digest{}) != newHash().Size() {
		panic("hash size mismatch")
	}
}

// Hasher is a file hasher.
type Hasher struct {
	h blake3.Hasher
	b [1024 * 1024]byte
}

// NewHasher returns a new file hasher.
func NewHasher() *Hasher {
	return &Hasher{h: *newHash()}
}

// Read computes the digest of the specified file.
func (h *Hasher) Read(fsys fs.FS, name string) (*File, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, fmt.Errorf("index: failed to open file: %s (%w)", name, err)
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("index: failed to stat file: %s (%w)", name, err)
	}
	h.h.Reset()
	n, err := io.CopyBuffer(&h.h, f, h.b[:])
	err2 := f.Close()
	if err != nil {
		return nil, fmt.Errorf("index: failed to read file: %s (%w)", name, err)
	}
	if err2 != nil {
		return nil, fmt.Errorf("index: failed to close file: %s (%w)", name, err2)
	}
	if n != fi.Size() {
		return nil, fmt.Errorf("index: file size mistmach: %s (want %d, got %d)", name, fi.Size(), n)
	}
	if fi2, err := fs.Stat(fsys, name); err != nil || fi.Size() != fi2.Size() || fi.ModTime() != fi2.ModTime() {
		return nil, fmt.Errorf("index: file modified while reading: %s", name)
	}
	file := &File{Path: filePath(name), Size: n, Mod: fi.ModTime()}
	h.h.Sum(file.Digest[:0])
	return file, nil
}
