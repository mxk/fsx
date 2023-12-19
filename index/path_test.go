package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilePath(t *testing.T) {
	assert.Equal(t, "a", filePath("a").String())
	assert.Equal(t, "a/b", filePath("a/b").String())
	assert.Panics(t, func() { filePath("") })
	assert.Panics(t, func() { filePath(".") })
	assert.Panics(t, func() { filePath("a/") })
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

func TestPathLess(t *testing.T) {
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
		assert.True(t, Path{tc.a}.less(Path{tc.b}), "%q", tc)
		assert.False(t, Path{tc.b}.less(Path{tc.a}), "%q", tc)
	}
	panics := []struct{ a, b string }{
		{"a/", "a"},
		{"a/a", "a"},
		{"a/b/c", "a/b"},
	}
	for _, tc := range panics {
		assert.Panics(t, func() { Path{tc.a}.less(Path{tc.b}) }, "%q", tc)
		assert.Panics(t, func() { Path{tc.b}.less(Path{tc.a}) }, "%q", tc)
	}
	assert.False(t, Root.less(Root))
	assert.False(t, Path{"a"}.less(Path{"a"}))
	assert.False(t, Path{"a/"}.less(Path{"a/"}))
	assert.False(t, Path{"a/b"}.less(Path{"a/b"}))
}
