package index

import (
	"context"
	"encoding/hex"
	"io/fs"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zeebo/blake3"
)

func TestHasher(t *testing.T) {
	// https://github.com/BLAKE3-team/BLAKE3/blob/master/test_vectors/test_vectors.json
	const testVec = "\x00\x01\x02"
	d1 := testDigest(t, "2d3adedff11b61f14c886e35afa036736dcd87a74d27b5c1510225d0f592e213")
	d2 := testDigest(t, "7b7015bb92cf0b318037702a6cdd81dee41224f734684c2c122cd6359cb1ee63")
	d3 := testDigest(t, "e1be4d7a8ab5560aa4199eea339849ba8e293d55ca0a81006726d184519e647f")
	d31744 := testDigest(t, "62b6960e1a44bcc1eb1a611a8d6235b6b4b78f32e7abc4fb4c6cdcce94895c47")
	v31744 := make([]byte, 31744)
	for i := range v31744 {
		v31744[i] = byte(i % 251)
	}

	t1 := time.Now()
	t2 := t1.Add(-time.Hour)
	fsys := fstest.MapFS{
		"a/b":       &fstest.MapFile{Data: []byte(testVec[:1]), ModTime: t1},
		testVec[:2]: &fstest.MapFile{ModTime: t1},
		"012":       &fstest.MapFile{Data: []byte(testVec), ModTime: t2},
		"~":         &fstest.MapFile{Data: v31744, ModTime: t2},
	}
	want := Files{
		&File{Path{"a/b"}, d1, 1, t1, flagNone},
		&File{Path{testVec[:2]}, d2, 0, t1, flagNone},
		&File{Path{"012"}, d3, 3, t2, flagNone},
		&File{Path{"~"}, d31744, 31744, t2, flagNone},
	}

	h := NewHasher(nil)
	var have Files
	err := fs.WalkDir(fsys, ".", func(name string, e fs.DirEntry, err error) error {
		if require.NoError(t, err); !e.IsDir() {
			f, err := h.Read(fsys, name, true)
			require.NoError(t, err)
			have = append(have, f)
		}
		return nil
	})
	require.NoError(t, err)
	have.Sort()
	require.Equal(t, want, have)
}

func TestHasherMonitor(t *testing.T) {
	var calls []int
	h := NewHasher(func(n int) error {
		if calls = append(calls, n); len(calls) > 2 {
			return context.Canceled
		}
		return nil
	})
	n := len(h.b)

	data := make([]byte, 3*n+1)
	fsys := fstest.MapFS{
		"a": &fstest.MapFile{Data: data[:n+1]},
		"b": &fstest.MapFile{Data: data},
		"c": &fstest.MapFile{},
	}

	f, err := h.Read(fsys, "a", false)
	require.NoError(t, err)
	require.Equal(t, Digest(blake3.Sum256(fsys["a"].Data)), f.digest)
	require.Equal(t, []int{n, 1}, calls)

	calls = calls[:0]
	_, err = h.Read(fsys, "b", false)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []int{n, n, n}, calls)

	calls = calls[:0]
	f, err = h.Read(fsys, "c", true)
	require.NoError(t, err)
	require.Equal(t, Digest(blake3.Sum256([]byte{'c'})), f.digest)
	require.Equal(t, []int{1}, calls)
}

func testDigest(t *testing.T, s string) (d Digest) {
	n, err := hex.Decode(d[:], []byte(s))
	require.NoError(t, err)
	require.Equal(t, len(d), n)
	return
}
