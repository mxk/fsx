package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlag(t *testing.T) {
	tests := []struct {
		s string
		f Flag
	}{
		{"", flagNone},
		{"D", flagDup},
		{"J", flagJunk},
		{"K", flagKeep},
		{"DX", flagDup | flagGone},
		{"JX", flagJunk | flagGone},
		{"KX", flagKeep | flagGone},
	}
	for _, tc := range tests {
		f, ok := parseFlag(tc.s)
		assert.True(t, ok)
		assert.Equal(t, tc.f, f, "%q", tc)
		assert.Equal(t, tc.s, tc.f.String(), "%q", tc)
	}

	_, ok := parseFlag("X")
	assert.False(t, ok)
	_, ok = parseFlag("XX")
	assert.False(t, ok)

	var f Flag
	assert.False(t, f.IsDup())
	assert.False(t, f.IsJunk())
	assert.False(t, f.Keep())
	assert.False(t, f.IsGone())
	assert.False(t, f.MayRemove())
	assert.True(t, f.IsSafe())
	assert.True(t, f.write())

	f = flagDup
	assert.True(t, f.IsDup())
	assert.False(t, f.IsJunk())
	assert.False(t, f.Keep())
	assert.False(t, f.IsGone())
	assert.True(t, f.MayRemove())
	assert.False(t, f.IsSafe())
	assert.True(t, f.write())

	f = flagKeep
	assert.False(t, f.IsDup())
	assert.False(t, f.IsJunk())
	assert.True(t, f.Keep())
	assert.False(t, f.IsGone())
	assert.False(t, f.MayRemove())
	assert.True(t, f.IsSafe())
	assert.True(t, f.write())

	f = flagGone
	assert.False(t, f.IsDup())
	assert.False(t, f.IsJunk())
	assert.False(t, f.Keep())
	assert.True(t, f.IsGone())
	assert.False(t, f.MayRemove())
	assert.False(t, f.IsSafe())
	assert.False(t, f.write())

	f = flagKeep | flagGone
	assert.False(t, f.IsDup())
	assert.False(t, f.IsJunk())
	assert.True(t, f.Keep())
	assert.True(t, f.IsGone())
	assert.False(t, f.MayRemove())
	assert.False(t, f.IsSafe())
	assert.True(t, f.write())
}
