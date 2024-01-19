package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTree(t *testing.T) {
	d1, d2, d3, d4, d5 := Digest{1}, Digest{2}, Digest{3}, Digest{4}, Digest{5}

	a1 := &File{Digest: d1, Path: Path{"a1"}}
	a2 := &File{Digest: d1, Path: Path{"A/a2"}}
	a3 := &File{Digest: d1, Path: Path{"A/B/a3"}}
	b1 := &File{Digest: d2, Path: Path{"A/b1"}}
	b2 := &File{Digest: d2, Path: Path{"C/D/E/b2"}}
	c1 := &File{Digest: d3, Path: Path{"C/c1"}}
	c2 := &File{Digest: d3, Path: Path{"C/F/c2"}}
	x1 := &File{Digest: d4, Path: Path{"C/.git/X/x1"}}
	y1 := &File{Digest: d5, Path: Path{"C/.git/X/.git/Z/y1"}}

	idx := Index{
		root:   "/",
		groups: []Files{{a1, a2, a3}, {b1, b2}, {c1, c2}, {x1}, {y1}},
	}

	Z := &Dir{
		Path:        Path{"C/.git/X/.git/Z/"},
		Files:       Files{y1},
		UniqueFiles: 1, // y1
	}
	GIT2 := &Dir{
		Path:        Path{"C/.git/X/.git/"},
		Dirs:        Dirs{Z},
		UniqueFiles: 1, // y1
	}
	X := &Dir{
		Path:        Path{"C/.git/X/"},
		Dirs:        Dirs{GIT2},
		Files:       Files{x1},
		UniqueFiles: 2, // x1, y1
	}
	GIT1 := &Dir{
		Path:        Path{"C/.git/"},
		Dirs:        Dirs{X},
		UniqueFiles: 2, // x1, y1
	}
	GIT1.Atom = GIT1
	X.Atom = GIT1
	GIT2.Atom = GIT1
	Z.Atom = GIT1

	F := &Dir{
		Path:        Path{"C/F/"},
		Files:       Files{c2},
		UniqueFiles: 1, // c2
	}
	E := &Dir{
		Path:        Path{"C/D/E/"},
		Files:       Files{b2},
		UniqueFiles: 1, // b2
	}
	D := &Dir{
		Path:        Path{"C/D/"},
		Dirs:        Dirs{E},
		UniqueFiles: 1, // b2,
	}
	C := &Dir{
		Path:        Path{"C/"},
		Dirs:        Dirs{GIT1, D, F},
		Files:       Files{c1},
		UniqueFiles: 4, // b2, c[12], x1, y1
	}
	B := &Dir{
		Path:        Path{"A/B/"},
		Files:       Files{a3},
		UniqueFiles: 1, // a3
	}
	A := &Dir{
		Path:        Path{"A/"},
		Dirs:        Dirs{B},
		Files:       Files{a2, b1},
		UniqueFiles: 2, // a[23], b1
	}
	root := &Dir{
		Path:        Root,
		Dirs:        Dirs{A, C},
		Files:       Files{a1},
		UniqueFiles: 5,
	}

	want := &Tree{
		root: idx.root,
		dirs: map[Path]*Dir{
			Root:      root,
			A.Path:    A,
			B.Path:    B,
			C.Path:    C,
			D.Path:    D,
			E.Path:    E,
			F.Path:    F,
			GIT1.Path: GIT1,
			X.Path:    X,
			GIT2.Path: GIT2,
			Z.Path:    Z,
		},
		idx: map[Digest]Files{
			d1: {a1, a2, a3},
			d2: {b1, b2},
			d3: {c1, c2},
			d4: {x1},
			d5: {y1},
		},
	}

	have := idx.Tree()
	mapEqual(t, want.dirs, have.dirs)
	mapEqual(t, want.idx, have.idx)
	assert.Equal(t, want, have)
}

func TestDedup(t *testing.T) {
	d1, d2, d3 := Digest{1}, Digest{2}, Digest{3}
	file := func(d Digest, p string) *File { return &File{Digest: d, Size: 1, Path: Path{p}} }

	a1 := file(d1, "A/a1")
	b1 := file(d2, "A/b1")
	c1 := file(d3, "A/c1")
	a2 := file(d1, "B/a2")
	b2 := file(d2, "B/b2")
	c2 := file(d3, "B/c2")

	idx := Index{
		root:   "/",
		groups: []Files{{a1, a2}, {b1, b2}, {c1, c2}},
	}

	tree := idx.Tree()
	want := []*Dup{{
		Dir: tree.dirs[Path{"A/"}],
		Alt: Dirs{tree.dirs[Path{"B/"}]},
	}, {
		Dir: tree.dirs[Path{"B/"}],
		Alt: Dirs{tree.dirs[Path{"A/"}]},
	}}

	assert.Equal(t, want, tree.Dups(Root, -1, 0))
}

func TestDirStack(t *testing.T) {
	var s dirStack
	assert.Nil(t, s.next())
	d1, d2, d3, d4 := new(Dir), new(Dir), new(Dir), new(Dir)

	s.from(d1)
	assert.Equal(t, d1, s.next())
	assert.Nil(t, s.next())

	s.from(d1, d2)
	assert.Equal(t, d1, s.next())
	assert.Equal(t, d2, s.next())
	assert.Nil(t, s.next())

	d1.Dirs = append(d1.Dirs, d2, d3)
	s.from(d1)
	assert.Equal(t, d1, s.next())
	assert.Equal(t, d2, s.next())
	assert.Equal(t, d3, s.next())
	assert.Nil(t, s.next())

	d2.Dirs = append(d2.Dirs, d4)
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
