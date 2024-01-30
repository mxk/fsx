package index

import (
	"context"
	"fmt"
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
		"X/a": {Data: b1, ModTime: t1},
		"X/b": {Data: b2, ModTime: t2},
		"Y/c": {Data: b1, ModTime: t2},
		"d":   {Data: b3, ModTime: t1},
	}
	x, err := Scan(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	want := &Index{groups: []Files{
		{
			{path{"X/a"}, d1, 1, t1, flagNone},
			{path{"Y/c"}, d1, 1, t2, flagNone},
		}, {
			{path{"X/b"}, d2, 2, t2, flagNone},
		}, {
			{path{"d"}, d3, 3, t1, flagNone},
		},
	}}
	require.Equal(t, want, x)

	// Remove, modify, and create files
	delete(fsys, "X/a")
	delete(fsys, "Y/c")
	fsys["X/b"].Data = b3
	fsys["e"] = &fstest.MapFile{Data: b1, ModTime: t2}

	// Rescan
	tr := x.ToTree()
	tr.file(path{"X/a"}).flag = flagJunk
	tr.file(path{"X/b"}).flag = flagKeep
	tr.file(path{"d"}).flag = flagDup
	x, err = tr.Rescan(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	want = &Index{groups: []Files{
		{
			{path{"X/a"}, d1, 1, t1, flagJunk | flagGone},
			{path{"e"}, d1, 1, t2, flagNone},
		}, {
			{path{"X/b"}, d3, 3, t2, flagNone},
			{path{"d"}, d3, 3, t1, flagDup | flagSame},
		}, {
			{path{"X/b"}, d2, 2, t2, flagKeep | flagGone},
		},
	}}
	require.Equal(t, want, x)

	// Restore original X/b and touch d
	fsys["X/b"].Data = b2
	fsys["d"].ModTime = t2

	// Rescan
	tr = x.ToTree()
	tr.file(path{"e"}).flag |= flagDup | flagGone
	x, err = tr.Rescan(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	want = &Index{groups: []Files{
		{
			{path{"X/a"}, d1, 1, t1, flagJunk | flagGone},
			{path{"e"}, d1, 1, t2, flagDup | flagSame},
		}, {
			{path{"X/b"}, d2, 2, t2, flagNone},
			{path{"X/b"}, d2, 2, t2, flagKeep | flagGone},
		}, {
			{path{"d"}, d3, 3, t2, flagNone},
			{path{"d"}, d3, 3, t1, flagDup | flagGone},
		},
	}}
	require.Equal(t, want, x)

	// Verify Tree structure
	X := &Dir{
		path:        path{"X/"},
		files:       Files{x.groups[1][0]},
		totalFiles:  1,
		uniqueFiles: 1,
	}
	R := &Dir{
		path:        root,
		dirs:        Dirs{X},
		files:       Files{x.groups[2][0], x.groups[0][1]},
		totalDirs:   1,
		totalFiles:  3,
		uniqueFiles: 3,
	}
	wantTree := &Tree{
		dirs: map[path]*Dir{root: R, X.path: X},
		idx: map[Digest]Files{
			d1: want.groups[0],
			d2: want.groups[1],
			d3: want.groups[2],
		},
	}
	require.Equal(t, wantTree, x.ToTree())

	// Rescan
	for _, g := range want.groups {
		for _, f := range g {
			if !f.flag.IsGone() {
				f.flag |= flagSame
			}
		}
	}
	x, err = x.ToTree().Rescan(context.Background(), fsys, nil, nil)
	require.NoError(t, err)
	require.Equal(t, want, x)
}

func TestProgress(t *testing.T) {
	t0 := time.Date(2006, 01, 02, 15, 04, 05, 00, time.UTC)
	p := newProgress(t0)
	want := "Indexed 0 files (0 B) in 0s [0 files/sec, 0 B/sec]"
	require.Equal(t, want, p.String())

	p.sampleFiles++
	p.sampleBytes.Add(128)
	p.update(t0.Add(time.Second))
	want = "Indexed 1 files (128 B) in 1s [1 files/sec, 128 B/sec]"
	require.Equal(t, want, p.String())

	p.sampleFiles++
	p.sampleBytes.Add(1024)
	p.update(t0.Add(2 * time.Second))
	want = fmt.Sprintf("Indexed 2 files (1.1 KiB) in 2s [1 files/sec, %.0f B/sec]", 0.9*128+0.1*1024)
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
