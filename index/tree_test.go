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
		Path:  Path{"C/.git/X/.git/Z/"},
		Files: Files{y1},
	}
	GIT2 := &Dir{
		Path: Path{"C/.git/X/.git/"},
		Dirs: Dirs{Z},
	}
	X := &Dir{
		Path:  Path{"C/.git/X/"},
		Dirs:  Dirs{GIT2},
		Files: Files{x1},
	}
	GIT1 := &Dir{
		Path: Path{"C/.git/"},
		Dirs: Dirs{X},
	}
	GIT1.Atom = GIT1
	X.Atom = GIT1
	GIT2.Atom = GIT1
	Z.Atom = GIT1

	F := &Dir{
		Path:  Path{"C/F/"},
		Files: Files{c2},
	}
	E := &Dir{
		Path:  Path{"C/D/E/"},
		Files: Files{b2},
	}
	D := &Dir{
		Path: Path{"C/D/"},
		Dirs: Dirs{E},
	}
	C := &Dir{
		Path:  Path{"C/"},
		Dirs:  Dirs{GIT1, D, F},
		Files: Files{c1},
	}
	B := &Dir{
		Path:  Path{"A/B/"},
		Files: Files{a3},
	}
	A := &Dir{
		Path:  Path{"A/"},
		Dirs:  Dirs{B},
		Files: Files{a2, b1},
	}
	root := &Dir{
		Path:  Root,
		Dirs:  Dirs{A, C},
		Files: Files{a1},
	}

	want := &Tree{
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
	for k, v := range want.dirs {
		require.Equal(t, v, have.dirs[k], "%s", k)
	}
	assert.Equal(t, want, have)
}
