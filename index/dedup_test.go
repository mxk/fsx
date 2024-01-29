package index

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestDedup(t *testing.T) {
	tree := func(fsys fstest.MapFS) *Tree {
		x, err := Scan(context.Background(), fsys, nil, nil)
		require.NoError(t, err)
		return x.ToTree()
	}

	var dd dedup
	tr := tree(fstest.MapFS{
		"A/B/a0":     {Data: []byte("a")},
		"A/B/b0":     {Data: []byte("b")},
		"A/B/C/c0":   {Data: []byte("c")},
		"A/B/C/D/d0": {Data: []byte("d")},
		"X/Y/a1":     {Data: []byte("a")},
		"X/Y/b1":     {Data: []byte("b")},
		"X/Y/e0":     {Data: []byte("e")},
		"X/Z/c1":     {Data: []byte("c")},
		"X/Z/d1":     {Data: []byte("d")},
		"X/Z/f0":     {Data: []byte("f")},
	})
	require.True(t, dd.isDup(tr, tr.Dir("A"), 0))
	require.True(t, dd.isDup(tr, tr.Dir("A/B"), 0))
	require.True(t, dd.isDup(tr, tr.Dir("A/B/C"), 0))
	require.True(t, dd.isDup(tr, tr.Dir("A/B/C/D"), 0))
	require.False(t, dd.isDup(tr, tr.Dir("X"), 1))
	require.False(t, dd.isDup(tr, tr.Dir("X/Y"), 0))
	require.True(t, dd.isDup(tr, tr.Dir("X/Z"), 1))
}
