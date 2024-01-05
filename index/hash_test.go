package index

import (
	"encoding/hex"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHasher(t *testing.T) {
	// https://github.com/BLAKE3-team/BLAKE3/blob/master/test_vectors/test_vectors.json
	const testVec = "\x00\x01\x02"
	d3 := testDigest(t, "e1be4d7a8ab5560aa4199eea339849ba8e293d55ca0a81006726d184519e647f")

	t1 := time.Now()
	t2 := t1.Add(-time.Hour)
	fsys := fstest.MapFS{
		testVec: &fstest.MapFile{ModTime: t1},
		"012":   &fstest.MapFile{Data: []byte(testVec), ModTime: t2},
	}

	h := NewHasher()
	have, err := h.Read(fsys, testVec)
	require.NoError(t, err)
	require.Equal(t, &File{Path{testVec}, d3, 0, t1}, have)

	have, err = h.Read(fsys, "012")
	require.NoError(t, err)
	require.Equal(t, &File{Path{"012"}, d3, 3, t2}, have)
}

func testDigest(t *testing.T, s string) (d Digest) {
	n, err := hex.Decode(d[:], []byte(s))
	require.NoError(t, err)
	require.Equal(t, len(d), n)
	return
}
