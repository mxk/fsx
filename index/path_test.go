package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPathLess(t *testing.T) {
	less := []struct{ a, b string }{
		{".", "!"},
		{".", "a/b"},
		{"a", "b"},
		{"a", "aa"},

		{"b/", "b/c"},
		{"b/c", "a"},
		{"b/", "a"},

		{"b/c", "ba"},
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
	}
	for _, tc := range panics {
		assert.Panics(t, func() { Path{tc.a}.less(Path{tc.b}) }, "%q", tc)
		assert.Panics(t, func() { Path{tc.b}.less(Path{tc.a}) }, "%q", tc)
	}
	assert.False(t, Root.less(Root))
	assert.False(t, Path{"a"}.less(Path{"a"}))
}
