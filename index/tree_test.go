package index

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToTree(t *testing.T) {
	d1, d2, d3, d4, d5 := Digest{1}, Digest{2}, Digest{3}, Digest{4}, Digest{5}

	a1 := &File{digest: d1, path: "a1"}
	a2 := &File{digest: d1, path: "A/a2"}
	a3 := &File{digest: d1, path: "A/B/a3"}
	b1 := &File{digest: d2, path: "A/b1"}
	b2 := &File{digest: d2, path: "C/D/E/b2"}
	c1 := &File{digest: d3, path: "C/c1"}
	c2 := &File{digest: d3, path: "C/F/c2"}
	x1 := &File{digest: d4, path: "C/.git/X/x1"}
	y1 := &File{digest: d5, path: "C/.git/X/.git/Z/y1"}
	yX := &File{digest: d5, path: "C/.git/X/.git/Z/y1", flag: flagGone}

	x := Index{
		root:   "/",
		groups: []Files{{a1, a2, a3}, {b1, b2}, {c1, c2}, {x1}, {y1, yX}},
	}

	Z := &dir{
		path:        "C/.git/X/.git/Z/",
		files:       Files{y1},
		totalFiles:  1,
		uniqueFiles: 1, // y1
	}
	GIT2 := &dir{
		path:        "C/.git/X/.git/",
		dirs:        dirs{Z},
		totalDirs:   1,
		totalFiles:  1,
		uniqueFiles: 1, // y1
	}
	X := &dir{
		path:        "C/.git/X/",
		dirs:        dirs{GIT2},
		files:       Files{x1},
		totalDirs:   2,
		totalFiles:  2,
		uniqueFiles: 2, // x1, y1
	}
	GIT1 := &dir{
		path:        "C/.git/",
		dirs:        dirs{X},
		totalDirs:   3,
		totalFiles:  2,
		uniqueFiles: 2, // x1, y1
	}
	GIT1.atom = GIT1
	X.atom = GIT1
	GIT2.atom = GIT1
	Z.atom = GIT1

	F := &dir{
		path:        "C/F/",
		files:       Files{c2},
		totalFiles:  1,
		uniqueFiles: 1, // c2
	}
	E := &dir{
		path:        "C/D/E/",
		files:       Files{b2},
		totalFiles:  1,
		uniqueFiles: 1, // b2
	}
	D := &dir{
		path:        "C/D/",
		dirs:        dirs{E},
		totalDirs:   1,
		totalFiles:  1,
		uniqueFiles: 1, // b2,
	}
	C := &dir{
		path:        "C/",
		dirs:        dirs{GIT1, D, F},
		files:       Files{c1},
		totalDirs:   7,
		totalFiles:  5,
		uniqueFiles: 4, // b2, c[12], x1, y1
	}
	B := &dir{
		path:        "A/B/",
		files:       Files{a3},
		totalFiles:  1,
		uniqueFiles: 1, // a3
	}
	A := &dir{
		path:        "A/",
		dirs:        dirs{B},
		files:       Files{a2, b1},
		totalDirs:   1,
		totalFiles:  3,
		uniqueFiles: 2, // a[23], b1
	}
	R := &dir{
		path:        ".",
		dirs:        dirs{A, C},
		files:       Files{a1},
		totalDirs:   10,
		totalFiles:  9,
		uniqueFiles: 5,
	}

	want := &Tree{
		root: x.root,
		dirs: map[path]*dir{
			R.path:    R,
			A.path:    A,
			B.path:    B,
			C.path:    C,
			D.path:    D,
			E.path:    E,
			F.path:    F,
			GIT1.path: GIT1,
			X.path:    X,
			GIT2.path: GIT2,
			Z.path:    Z,
		},
		idx: map[Digest]Files{
			d1: {a1, a2, a3},
			d2: {b1, b2},
			d3: {c1, c2},
			d4: {x1},
			d5: {y1, yX},
		},
	}

	have := x.ToTree()
	mapEqual(t, want.dirs, have.dirs)
	mapEqual(t, want.idx, have.idx)
	assert.Equal(t, want, have)
}

func TestEmptyTree(t *testing.T) {
	want := &Tree{root: "/", dirs: map[path]*dir{".": {path: "."}}}
	require.Equal(t, want, (&Index{root: "/"}).ToTree())
	require.Equal(t, &Index{root: "/"}, want.ToIndex())

	d1 := Digest{1}
	x := &Index{root: "/", groups: []Files{{
		{"x", d1, 1, time.Time{}, flagDup | flagGone},
	}}}
	want.idx = map[Digest]Files{d1: x.groups[0]}
	require.Equal(t, want, x.ToTree())
	require.Equal(t, x, want.ToIndex())
}

func TestToIndex(t *testing.T) {
	x, err := read(strings.NewReader(testIdx))
	require.NoError(t, err)
	require.Equal(t, x, x.ToTree().ToIndex())
}

func TestDups(t *testing.T) {
	d1, d2, d3 := Digest{1}, Digest{2}, Digest{3}
	file := func(d Digest, p path) *File { return &File{digest: d, size: 1, path: p} }

	a0, a1 := file(d1, "A/a0"), file(d1, "B/a1")
	b0, b1 := file(d2, "A/b0"), file(d2, "B/b1")
	c0, c1 := file(d3, "A/c0"), file(d3, "B/c1")
	c1.flag |= flagGone

	x := Index{groups: []Files{{a0, a1}, {b0, b1}, {c0, c1}}}
	tr := x.ToTree()
	want := []*Dup{{
		dir:  tr.dirs["A/"],
		Alt:  dirs{tr.dirs["B/"]},
		Lost: Files{c0},
	}, {
		dir: tr.dirs["B/"],
		Alt: dirs{tr.dirs["A/"]},
	}}
	require.Equal(t, want, tr.Dups(".", -1, 1))

	a0.flag |= flagKeep
	require.Equal(t, want[1:], tr.Dups(".", -1, 1))

	a0.flag = flagNone
	c0.flag |= flagKeep | flagGone
	a1.flag |= flagKeep
	want[0].Lost = nil
	require.Equal(t, want[:1], tr.Dups(".", -1, 1))
}

func TestDirStack(t *testing.T) {
	var s dirStack
	assert.Nil(t, s.next())
	d1, d2, d3, d4 := new(dir), new(dir), new(dir), new(dir)

	s.from(d1)
	assert.Equal(t, d1, s.next())
	assert.Nil(t, s.next())

	s.from(d1, d2)
	assert.Equal(t, d1, s.next())
	assert.Equal(t, d2, s.next())
	assert.Nil(t, s.next())

	d1.dirs = append(d1.dirs, d2, d3)
	s.from(d1)
	assert.Equal(t, d1, s.next())
	assert.Equal(t, d2, s.next())
	assert.Equal(t, d3, s.next())
	assert.Nil(t, s.next())

	d2.dirs = append(d2.dirs, d4)
	s.from(d1)
	assert.Equal(t, d1, s.next())
	assert.Equal(t, d2, s.next())
	assert.Equal(t, d4, s.next())
	assert.Equal(t, d3, s.next())
	assert.Nil(t, s.next())
}

func mapEqual[K comparable, V any](t *testing.T, want, have map[K]V) {
	for k, v := range want {
		require.Equal(t, v, have[k], "%+v", k)
	}
	for k, v := range have {
		require.Equal(t, want[k], v, "%+v", k)
	}
}
