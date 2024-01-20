package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuild(t *testing.T) {
	const testVec = "\x00\x01"
	d1 := testDigest(t, "2d3adedff11b61f14c886e35afa036736dcd87a74d27b5c1510225d0f592e213")
	d2 := testDigest(t, "7b7015bb92cf0b318037702a6cdd81dee41224f734684c2c122cd6359cb1ee63")
	t1 := time.Now()
	t2 := t1.Add(-time.Hour)
	fsys := fstest.MapFS{
		"b/c": &fstest.MapFile{Data: []byte(testVec[:1]), ModTime: t1},
		"b/d": &fstest.MapFile{Data: []byte(testVec[:2]), ModTime: t2},
		"a":   &fstest.MapFile{Data: []byte(testVec[:1]), ModTime: t2},
	}
	idx, err := Build(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	want := Index{
		groups: []Files{{
			{Path{"b/c"}, d1, 1, t1, flagNone},
			{Path{"a"}, d1, 1, t2, flagNone},
		}, {
			{Path{"b/d"}, d2, 2, t2, flagNone},
		}},
	}
	assert.Equal(t, want, idx)
}

func TestProgress(t *testing.T) {
	var p Progress
	want := "Indexed 0 files (0 B) in 0s [0 files/sec, 0 B/sec]"
	require.Equal(t, want, p.String())

	t0 := time.Date(2006, 01, 02, 15, 04, 05, 00, time.UTC)
	p.reset(t0)
	p.fileDone(t0.Add(time.Second), 128)
	want = "Indexed 1 files (128 B) in 1s [1 files/sec, 128 B/sec]"
	require.Equal(t, want, p.String())

	p.fileDone(t0.Add(2*time.Second), 1024)
	want = "Indexed 2 files (1.1 KiB) in 2s [1 files/sec, 143 B/sec]"
	require.Equal(t, want, p.String())
}

func TestDirFSRoot(t *testing.T) {
	want := filepath.Clean(os.TempDir())
	assert.Equal(t, want, dirFSRoot(os.DirFS(want)))

	fsys := fstest.MapFS(nil)
	_, err := fsys.Open(".")
	assert.NoError(t, err)
	assert.Empty(t, dirFSRoot(fsys))
}
