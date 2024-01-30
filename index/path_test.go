package index

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirPath(t *testing.T) {
	assert.Equal(t, ".", dirPath(".").String())
	assert.Equal(t, ".", dirPath("a/..").String())
	assert.Equal(t, "a/", dirPath("./a").String())
	assert.Equal(t, "a/b/", dirPath("a/b/").String())
	for _, tc := range []string{"", "/", "..", "a/../..", "C:Windows", "C:/Windows", "//x/y"} {
		assert.Panics(t, func() { dirPath(tc) }, "%q", tc)
	}
}

func TestStrictFilePath(t *testing.T) {
	for _, tc := range []string{"a", "a/b"} {
		assert.Equal(t, tc, strictFilePath(tc).String(), "%q", tc)
	}
	for _, tc := range []string{"", ".", "/a", "a/", "./a", "a/./b"} {
		assert.Panics(t, func() { strictFilePath(tc) }, "%q", tc)
	}
}

func TestPathContains(t *testing.T) {
	assert.True(t, root.contains(root))
	assert.True(t, root.contains(path{"a"}))
	assert.True(t, root.contains(path{"a/"}))
	assert.False(t, path{}.contains(root))
	assert.False(t, path{}.contains(path{"a"}))
	assert.False(t, path{"a"}.contains(path{"a"}))
	assert.False(t, path{"a/a"}.contains(path{"a/a"}))
	assert.False(t, path{"a/"}.contains(root))
	assert.False(t, path{"a/"}.contains(path{"a"}))
	assert.False(t, path{"a/"}.contains(path{"b"}))
	assert.False(t, path{"a/"}.contains(path{"b/"}))
	assert.False(t, path{"a/b"}.contains(path{"a/"}))
	assert.True(t, path{"a/"}.contains(path{"a/"}))
	assert.True(t, path{"a/"}.contains(path{"a/b"}))
}

func TestPathDirBase(t *testing.T) {
	tests := []struct{ p, dir, base string }{
		{".", ".", "."},
		{"a", ".", "a"},
		{"a/", ".", "a"},
		{"a/b", "a/", "b"},
		{"a/b/", "a/", "b"},
		{"a/bc/de", "a/bc/", "de"},
		{"a/bc/de/", "a/bc/", "de"},
	}
	for _, tc := range tests {
		assert.Equal(t, path{tc.dir}, path{tc.p}.dir(), "%q", tc)
		assert.Equal(t, tc.base, path{tc.p}.base(), "%q", tc)
	}
	assert.Panics(t, func() { path{"/a"}.dir() })
}

func TestPathCommonRoot(t *testing.T) {
	tests := []struct {
		a, b, root string
	}{
		{".", ".", "."},
		{"a/", ".", "."},
		{"a/", "a/", "a/"},
		{"a/b", "a/", "a/"},
		{"a/b", "a/c", "a/"},
		{"a/b/", "a/", "a/"},
		{"a/", "b/", "."},
		{"a/b/", "a/c/", "a/"},
		{"a/b/", "b/c/", "."},
		{"a/b/c/", "a/b/d", "a/b/"},
		{"a/b/c/", "a/b/d/", "a/b/"},
	}
	for _, tc := range tests {
		assert.Equal(t, path{tc.root}, path{tc.a}.commonRoot(path{tc.b}), "%q", tc)
		assert.Equal(t, path{tc.root}, path{tc.b}.commonRoot(path{tc.a}), "%q", tc)
	}
}

func TestPathDist(t *testing.T) {
	tests := []struct {
		a, b string
		dist int
	}{
		{".", ".", 0},
		{"a", ".", 0},
		{"a/", ".", 1},
		{"a/", "a/", 0},
		{"a/b", "a/", 0},
		{"a/b", "a/c", 0},
		{"a/b/", "a/", 1},
		{"a/", "b/", 2},
		{"a/b/", "a/c/", 2},
		{"a/b/", "b/c/", 4},
		{"a/b/c/", "a/b/d", 1},
		{"a/b/c/", "a/b/d/", 2},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.dist, path{tc.a}.dist(path{tc.b}), "%q", tc)
		assert.Equal(t, tc.dist, path{tc.b}.dist(path{tc.a}), "%q", tc)
	}
}

func TestPathIsDir(t *testing.T) {
	assert.False(t, path{}.isDir())
	assert.True(t, root.isDir())
	assert.False(t, path{"a"}.isDir())
	assert.True(t, path{"a/"}.isDir())
}

func TestPathIsFile(t *testing.T) {
	assert.False(t, path{}.isFile())
	assert.False(t, root.isFile())
	assert.True(t, path{"a"}.isFile())
	assert.False(t, path{"a/"}.isFile())
}

func TestPathCmp(t *testing.T) {
	less := []struct{ a, b string }{
		{".", "!"},
		{".", "a/b"},
		{"a", "b"},
		{"a", "aa"},

		{"b/", "b/c"},
		{"b/c", "a"},
		{"b/", "a"},

		{"b/", "ba"},
		{"b/c", "ba"},
		{"b/c", "ba/"},
		{"b/c", "b/ca"},
		{"a/b", "aa/b"},
		{"aa/b", "a"},
		{"a/a/", "a/ab/"},
		{"a/ab/", "a/a"},
	}
	for _, tc := range less {
		assert.Equal(t, -1, path{tc.a}.cmp(path{tc.b}), "%q", tc)
		assert.Equal(t, 1, path{tc.b}.cmp(path{tc.a}), "%q", tc)
	}
	panics := []struct{ a, b string }{
		{"", "."},
		{"", "a"},
		{"a/", "a"},
		{"a/a", "a"},
		{"a/b/c", "a/b"},
	}
	for _, tc := range panics {
		assert.Panics(t, func() { path{tc.a}.cmp(path{tc.b}) }, "%q", tc)
		assert.Panics(t, func() { path{tc.b}.cmp(path{tc.a}) }, "%q", tc)
	}
	assert.Zero(t, root.cmp(root))
	assert.Zero(t, path{"a"}.cmp(path{"a"}))
	assert.Zero(t, path{"a/"}.cmp(path{"a/"}))
	assert.Zero(t, path{"a/b"}.cmp(path{"a/b"}))
}

func TestSteps(t *testing.T) {
	tests := []struct {
		path string
		skip string
		want []string
	}{
		// next
		{"", "", nil},
		{".", "", nil},
		{"a", "", []string{"a"}},
		{"a/", "", []string{"a/"}},
		{"a/b", "", []string{"a/", "a/b"}},
		{"a/bc/", "", []string{"a/", "a/bc/"}},
		{"a/bc/def/ghi", "", []string{"a/", "a/bc/", "a/bc/def/", "a/bc/def/ghi"}},

		// skip
		{"a", "a", []string{"a"}},
		{"a", "a/", []string{"a"}},
		{"a/b/c", "x/", []string{"a/", "a/b/", "a/b/c"}},
		{"a/b/c", "a/", []string{"a/b/", "a/b/c"}},
		{"a/b/c", "a/b", []string{"a/", "a/b/", "a/b/c"}},
		{"a/b/c/", "a/b/", []string{"a/b/c/"}},
		{"a/b/c/", "a/b/c/", nil},
	}
	for _, tc := range tests {
		s := steps{path: path{tc.path}}
		if tc.skip != "" {
			s.skip(path{tc.skip})
		}
		var have []string
		for p, ok := s.next(); ok; p, ok = s.next() {
			have = append(have, p.p)
		}
		assert.Equal(t, tc.want, have, "%q", tc)
	}
	for i := range [3]struct{}{} {
		s := steps{path: path{"a/b/c/d/"}}
		if p, ok := s.next(); assert.True(t, ok) {
			assert.Equal(t, path{"a/"}, p)
		}
		switch i {
		case 0:
			s.skip(path{"a/b/c/"})
		case 1:
			s.skip(path{"a/b/"})
			s.skip(path{"a/b/c/"})
		case 2:
			s.skip(path{"a/b/c/"})
			s.skip(path{"a/b/c/d/e/"})
			s.skip(path{"a/b/"})
		}
		if p, ok := s.next(); assert.True(t, ok, "%v", i) {
			assert.Equal(t, path{"a/b/c/d/"}, p, "%v", i)
		}
	}
	assert.Panics(t, func() { (&steps{path: path{"/a"}}).next() })
	assert.Panics(t, func() {
		s := steps{path: path{"a//b"}}
		if p, ok := s.next(); assert.True(t, ok) {
			assert.Equal(t, path{"a/"}, p)
		}
		s.next()
	})
}

func TestUniqueDirs(t *testing.T) {
	var have []path
	fn := func(p path) { have = append(have, p) }

	var u uniqueDirs
	u.forEach(func(path) { panic("fail") })

	u.add(dirPath("A/"))
	u.forEach(fn)
	require.Equal(t, []path{root, {"A/"}}, have)
	require.Empty(t, u)

	u.add(dirPath("A/"))
	u.add(dirPath("X/Y/Z/"))
	u.add(dirPath("A/B/C/D/"))
	u.add(dirPath("A/B/C/"))
	u.add(dirPath("X/Z/"))
	u.add(root)
	u.add(dirPath("A/B/"))
	u.add(dirPath("A/B/E/"))
	u.add(dirPath("A/B/C/D/"))

	want := []path{
		root,
		{"A/"},
		{"X/"},
		{"X/Y/"},
		{"X/Y/Z/"},
		{"A/B/"},
		{"A/B/C/"},
		{"A/B/C/D/"},
		{"X/Z/"},
		{"A/B/E/"},
	}
	have = have[:0]
	u.forEach(fn)
	require.Equal(t, want, have)
	require.Empty(t, u)
}

func TestCleanPath(t *testing.T) {
	tests := []struct{ have, want string }{
		{"", ""},
		{"/", ""},
		{`\\`, ""},
		{"C:", ""},
		{"a/../c:", ""},
		{"a/../..", ""},
		{"../a", ""},
		{"./", "."},
		{"./a/", "a/"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, cleanPath(tc.have), "%q", tc)
	}

	// Do not reallocate clean paths
	for _, tc := range []string{".", "a", "a/", "a/b", "a/b/"} {
		if p := cleanPath(tc); assert.Equal(t, tc, p) {
			assert.Same(t, unsafe.StringData(tc), unsafe.StringData(p))
		}
	}
}
