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
	assert.Panics(t, func() { _ = flagGone.String() })

	assert.True(t, flagDup.IsDup())
	assert.False(t, flagDup.IsJunk())
	assert.False(t, flagDup.Keep())
	assert.False(t, flagDup.IsGone())
	assert.True(t, flagDup.MayRemove())

	a := flagKeep | flagGone
	assert.False(t, a.IsDup())
	assert.False(t, a.IsJunk())
	assert.True(t, a.Keep())
	assert.True(t, a.IsGone())
	assert.False(t, a.MayRemove())
}
