package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilePath(t *testing.T) {
	assert.Equal(t, "a", filePath("a").String())
	assert.Equal(t, "a/b", filePath("a/b").String())
	assert.Panics(t, func() { filePath("") })
	assert.Panics(t, func() { filePath(".") })
	assert.Panics(t, func() { filePath("a/") })
}

func TestDirPath(t *testing.T) {
	assert.Equal(t, ".", dirPath(".").String())
	assert.Equal(t, "a/", dirPath("./a").String())
	assert.Equal(t, "a/b/", dirPath("a/b/").String())
}

func TestPathIsDir(t *testing.T) {
	assert.True(t, Root.IsDir())
	assert.False(t, Path{"a"}.IsDir())
	assert.True(t, Path{"a/"}.IsDir())
}

func TestPathContains(t *testing.T) {
	assert.True(t, Root.Contains(Root))
	assert.True(t, Root.Contains(Path{"a"}))
	assert.True(t, Root.Contains(Path{"a/"}))
	assert.False(t, Path{}.Contains(Root))
	assert.False(t, Path{}.Contains(Path{"a"}))
	assert.False(t, Path{"a"}.Contains(Path{"a"}))
	assert.False(t, Path{"a/a"}.Contains(Path{"a/a"}))
	assert.False(t, Path{"a/"}.Contains(Root))
	assert.False(t, Path{"a/"}.Contains(Path{"a"}))
	assert.False(t, Path{"a/"}.Contains(Path{"b"}))
	assert.False(t, Path{"a/"}.Contains(Path{"b/"}))
	assert.False(t, Path{"a/b"}.Contains(Path{"a/"}))
	assert.True(t, Path{"a/"}.Contains(Path{"a/"}))
	assert.True(t, Path{"a/"}.Contains(Path{"a/b"}))
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
		assert.Equal(t, Path{tc.dir}, Path{tc.p}.Dir(), "%q", tc)
		assert.Equal(t, tc.base, Path{tc.p}.Base(), "%q", tc)
	}
	assert.Panics(t, func() { Path{"/a"}.Dir() })
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
		assert.Equal(t, Path{tc.root}, Path{tc.a}.CommonRoot(Path{tc.b}), "%q", tc)
		assert.Equal(t, Path{tc.root}, Path{tc.b}.CommonRoot(Path{tc.a}), "%q", tc)
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
		assert.Equal(t, tc.dist, Path{tc.a}.Dist(Path{tc.b}), "%q", tc)
		assert.Equal(t, tc.dist, Path{tc.b}.Dist(Path{tc.a}), "%q", tc)
	}
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
		assert.Equal(t, -1, Path{tc.a}.cmp(Path{tc.b}), "%q", tc)
		assert.Equal(t, 1, Path{tc.b}.cmp(Path{tc.a}), "%q", tc)
	}
	panics := []struct{ a, b string }{
		{"", "."},
		{"", "a"},
		{"a/", "a"},
		{"a/a", "a"},
		{"a/b/c", "a/b"},
	}
	for _, tc := range panics {
		assert.Panics(t, func() { Path{tc.a}.cmp(Path{tc.b}) }, "%q", tc)
		assert.Panics(t, func() { Path{tc.b}.cmp(Path{tc.a}) }, "%q", tc)
	}
	assert.Zero(t, Root.cmp(Root))
	assert.Zero(t, Path{"a"}.cmp(Path{"a"}))
	assert.Zero(t, Path{"a/"}.cmp(Path{"a/"}))
	assert.Zero(t, Path{"a/b"}.cmp(Path{"a/b"}))
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
		s := steps{Path: Path{tc.path}}
		if tc.skip != "" {
			s.skip(Path{tc.skip})
		}
		var have []string
		for p, ok := s.next(); ok; p, ok = s.next() {
			have = append(have, p.p)
		}
		assert.Equal(t, tc.want, have, "%q", tc)
	}
	for i := range [3]struct{}{} {
		s := steps{Path: Path{"a/b/c/d/"}}
		if p, ok := s.next(); assert.True(t, ok) {
			assert.Equal(t, Path{"a/"}, p)
		}
		switch i {
		case 0:
			s.skip(Path{"a/b/c/"})
		case 1:
			s.skip(Path{"a/b/"})
			s.skip(Path{"a/b/c/"})
		case 2:
			s.skip(Path{"a/b/c/"})
			s.skip(Path{"a/b/c/d/e/"})
			s.skip(Path{"a/b/"})
		}
		if p, ok := s.next(); assert.True(t, ok, "%v", i) {
			assert.Equal(t, Path{"a/b/c/d/"}, p, "%v", i)
		}
	}
	assert.Panics(t, func() { (&steps{Path: Path{"/a"}}).next() })
	assert.Panics(t, func() {
		s := steps{Path: Path{"a//b"}}
		if p, ok := s.next(); assert.True(t, ok) {
			assert.Equal(t, Path{"a/"}, p)
		}
		s.next()
	})
}

func TestUniqueDirs(t *testing.T) {
	var have []Path
	fn := func(p Path) { have = append(have, p) }

	var u uniqueDirs
	u.forEach(func(Path) { panic("fail") })

	u.add(dirPath("A/"))
	u.forEach(fn)
	require.Equal(t, []Path{Root, {"A/"}}, have)
	require.Empty(t, u)

	u.add(dirPath("A/"))
	u.add(dirPath("X/Y/Z/"))
	u.add(dirPath("A/B/C/D/"))
	u.add(dirPath("A/B/C/"))
	u.add(dirPath("X/Z/"))
	u.add(Root)
	u.add(dirPath("A/B/"))
	u.add(dirPath("A/B/E/"))
	u.add(dirPath("A/B/C/D/"))

	want := []Path{
		Root,
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
