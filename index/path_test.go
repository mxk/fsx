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
	assert.False(t, path("").contains(""))
	assert.False(t, path("").contains("."))
	assert.False(t, path("").contains("a"))

	assert.True(t, path(".").contains("."))
	assert.True(t, path(".").contains("a"))
	assert.True(t, path(".").contains("a/"))

	assert.False(t, path("a").contains("a"))
	assert.False(t, path("a/a").contains("a/a"))
	assert.False(t, path("a/").contains("."))
	assert.False(t, path("a/").contains("a"))
	assert.False(t, path("a/").contains("b"))
	assert.False(t, path("a/").contains("b/"))
	assert.False(t, path("a/b").contains("a/"))

	assert.True(t, path("a/").contains("a/"))
	assert.True(t, path("a/").contains("a/b"))
}

func TestPathDirBase(t *testing.T) {
	tests := []struct {
		p, dir path
		base   string
	}{
		{".", ".", "."},
		{"a", ".", "a"},
		{"a/", ".", "a"},
		{"a/b", "a/", "b"},
		{"a/b/", "a/", "b"},
		{"a/bc/de", "a/bc/", "de"},
		{"a/bc/de/", "a/bc/", "de"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.dir, tc.p.dir(), "%q", tc)
		assert.Equal(t, tc.base, tc.p.base(), "%q", tc)
	}
	assert.PanicsWithValue(t, emptyPath, func() { path("").dir() })
	assert.PanicsWithValue(t, emptyPath, func() { path("").base() })
}

func TestPathCommonRoot(t *testing.T) {
	tests := []struct{ a, b, root path }{
		{".", ".", "."},
		{"a", ".", "."},
		{"a", "a", "."},
		{"a", "b", "."},
		{"a/", ".", "."},
		{"a/", "a", "."},
		{"a/", "a/", "a/"},
		{"a/b", "a/", "a/"},
		{"a/b", "a/c", "a/"},
		{"a/b/", "a/", "a/"},
		{"a/", "b", "."},
		{"a/", "b/", "."},
		{"a/b/", "a/c/", "a/"},
		{"a/b/", "b/c/", "."},
		{"a/b/c/", "a/b/d", "a/b/"},
		{"a/b/c/", "a/b/d/", "a/b/"},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.root, tc.a.commonRoot(tc.b), "%q", tc)
		assert.Equal(t, tc.root, tc.b.commonRoot(tc.a), "%q", tc)
	}
	assert.PanicsWithValue(t, emptyPath, func() { path("").commonRoot("") })
	assert.PanicsWithValue(t, emptyPath, func() { path("").commonRoot(".") })
}

func TestPathDist(t *testing.T) {
	tests := []struct {
		a, b path
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
		assert.Equal(t, tc.dist, tc.a.dist(tc.b), "%q", tc)
		assert.Equal(t, tc.dist, tc.b.dist(tc.a), "%q", tc)
	}
}

func TestPathIsDir(t *testing.T) {
	assert.False(t, path("").isDir())
	assert.True(t, path(".").isDir())
	assert.False(t, path("a").isDir())
	assert.True(t, path("a/").isDir())
}

func TestPathIsFile(t *testing.T) {
	assert.False(t, path("").isFile())
	assert.False(t, path(".").isFile())
	assert.True(t, path("a").isFile())
	assert.False(t, path("a/").isFile())
}

func TestPathCmp(t *testing.T) {
	less := []struct{ a, b path }{
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
		assert.Equal(t, -1, tc.a.cmp(tc.b), "%q", tc)
		assert.Equal(t, 1, tc.b.cmp(tc.a), "%q", tc)
	}
	panics := []struct{ a, b path }{
		{"", ""},
		{"", "."},
		{"", "a"},
		{"a/", "a"},
		{"a/a", "a"},
		{"a/b/c", "a/b"},
	}
	for _, tc := range panics {
		assert.Panics(t, func() { tc.a.cmp(tc.b) }, "%q", tc)
		assert.Panics(t, func() { tc.b.cmp(tc.a) }, "%q", tc)
	}
	assert.Zero(t, path(".").cmp("."))
	assert.Zero(t, path("a").cmp("a"))
	assert.Zero(t, path("a/").cmp("a/"))
	assert.Zero(t, path("a/b").cmp("a/b"))
}

func TestSteps(t *testing.T) {
	tests := []struct {
		path path
		skip path
		want []path
	}{
		// next
		{"", "", nil},
		{".", "", nil},
		{"a", "", []path{"a"}},
		{"a/", "", []path{"a/"}},
		{"a/b", "", []path{"a/", "a/b"}},
		{"a/bc/", "", []path{"a/", "a/bc/"}},
		{"a/bc/def/ghi", "", []path{"a/", "a/bc/", "a/bc/def/", "a/bc/def/ghi"}},

		// skip
		{"a", "a", []path{"a"}},
		{"a", "a/", []path{"a"}},
		{"a/b/c", "x/", []path{"a/", "a/b/", "a/b/c"}},
		{"a/b/c", "a/", []path{"a/b/", "a/b/c"}},
		{"a/b/c", "a/b", []path{"a/", "a/b/", "a/b/c"}},
		{"a/b/c/", "a/b/", []path{"a/b/c/"}},
		{"a/b/c/", "a/b/c/", nil},
	}
	for _, tc := range tests {
		s := steps{p: tc.path}
		if tc.skip != "" {
			s.skip(tc.skip)
		}
		var have []path
		for p := s.next(); p != ""; p = s.next() {
			have = append(have, p)
		}
		assert.Equal(t, tc.want, have, "%q", tc)
	}
	for i := range [3]struct{}{} {
		s := steps{p: "a/b/c/d/"}
		assert.Equal(t, path("a/"), s.next())
		switch i {
		case 0:
			s.skip("a/b/c/")
		case 1:
			s.skip("a/b/")
			s.skip("a/b/c/")
		case 2:
			s.skip("a/b/c/")
			s.skip("a/b/c/d/e/")
			s.skip("a/b/")
		}
		assert.Equal(t, path("a/b/c/d/"), s.next(), "%v", i)
	}
	assert.Panics(t, func() { (&steps{p: "/a"}).next() })
	assert.Panics(t, func() {
		s := steps{p: "a//b"}
		assert.Equal(t, path("a/"), s.next())
		s.next()
	})
}

func TestUniqueDirs(t *testing.T) {
	var have []path
	fn := func(p path) { have = append(have, p) }

	var u uniqueDirs
	u.forEach(func(path) { panic("fail") })
	assert.Panics(t, func() { u.add("") })
	assert.Panics(t, func() { u.add("a") })

	u.add("A/")
	u.forEach(fn)
	require.Equal(t, []path{".", "A/"}, have)
	require.Empty(t, u)

	u.add("A/")
	u.add("X/Y/Z/")
	u.add("A/B/C/D/")
	u.add("A/B/C/")
	u.add("X/Z/")
	u.add(".")
	u.add("A/B/")
	u.add("A/B/E/")
	u.add("A/B/C/D/")

	want := []path{
		".",
		"A/",
		"X/",
		"X/Y/",
		"X/Y/Z/",
		"A/B/",
		"A/B/C/",
		"A/B/C/D/",
		"X/Z/",
		"A/B/E/",
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
