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
	"github.com/zeebo/blake3"
)

func TestScan(t *testing.T) {
	b1, d1 := testData("\x00")
	b2, d2 := testData("\x00\x01")
	b3, d3 := testData("\x00\x01\x02")
	t1 := time.Now()
	t2 := t1.Add(-time.Hour)
	fsys := fstest.MapFS{
		"X/a": &fstest.MapFile{Data: b1, ModTime: t1},
		"X/b": &fstest.MapFile{Data: b2, ModTime: t2},
		"Y/c": &fstest.MapFile{Data: b1, ModTime: t2},
		"d":   &fstest.MapFile{Data: b3, ModTime: t1},
	}
	idx, err := Scan(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	want := Index{groups: []Files{
		{
			{Path{"X/a"}, d1, 1, t1, flagNone},
			{Path{"Y/c"}, d1, 1, t2, flagNone},
		}, {
			{Path{"X/b"}, d2, 2, t2, flagNone},
		}, {
			{Path{"d"}, d3, 3, t1, flagNone},
		},
	}}
	require.Equal(t, want, idx)

	// Remove, modify, and create files
	delete(fsys, "X/a")
	delete(fsys, "Y/c")
	fsys["X/b"].Data = b3
	fsys["e"] = &fstest.MapFile{Data: b1, ModTime: t2}

	// Rescan
	tr := idx.ToTree()
	tr.file(Path{"X/a"}).flag = flagJunk
	tr.file(Path{"X/b"}).flag = flagKeep
	tr.file(Path{"d"}).flag = flagDup
	idx, err = tr.Rescan(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	want = Index{groups: []Files{
		{
			{Path{"X/a"}, d1, 1, t1, flagJunk | flagGone},
			{Path{"e"}, d1, 1, t2, flagNone},
		}, {
			{Path{"X/b"}, d3, 3, t2, flagNone},
			{Path{"d"}, d3, 3, t1, flagDup | flagSame},
		}, {
			{Path{"X/b"}, d2, 2, t2, flagKeep | flagGone},
		},
	}}
	require.Equal(t, want, idx)

	// Restore original X/b and touch d
	fsys["X/b"].Data = b2
	fsys["d"].ModTime = t2

	// Rescan
	tr = idx.ToTree()
	tr.file(Path{"e"}).flag |= flagDup | flagGone
	idx, err = tr.Rescan(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	want = Index{groups: []Files{
		{
			{Path{"X/a"}, d1, 1, t1, flagJunk | flagGone},
			{Path{"e"}, d1, 1, t2, flagDup | flagSame},
		}, {
			{Path{"X/b"}, d2, 2, t2, flagNone},
			{Path{"X/b"}, d2, 2, t2, flagKeep | flagGone},
		}, {
			{Path{"d"}, d3, 3, t2, flagNone},
			{Path{"d"}, d3, 3, t1, flagDup | flagGone},
		},
	}}
	require.Equal(t, want, idx)

	// Verify Tree structure
	X := &Dir{
		Path:        Path{"X/"},
		files:       Files{idx.groups[1][0]},
		uniqueFiles: 1,
	}
	root := &Dir{
		Path:        Root,
		dirs:        Dirs{X},
		files:       Files{idx.groups[2][0], idx.groups[0][1]},
		uniqueFiles: 3,
	}
	wantTree := &Tree{
		dirs: map[Path]*Dir{Root: root, X.Path: X},
		idx: map[Digest]Files{
			d1: want.groups[0],
			d2: want.groups[1],
			d3: want.groups[2],
		},
	}
	require.Equal(t, wantTree, idx.ToTree())
}

func TestProgress(t *testing.T) {
	t0 := time.Date(2006, 01, 02, 15, 04, 05, 00, time.UTC)
	p := newProgress(t0)
	want := "Indexed 0 files (0 B) in 0s [0 files/sec, 0 B/sec]"
	require.Equal(t, want, p.String())

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

func testData(s string) ([]byte, Digest) {
	b := []byte(s)
	return b, blake3.Sum256(b)
}
