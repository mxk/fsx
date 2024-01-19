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
		{"X", flagGone},
		{"DX", flagDup | flagGone},
		{"JX", flagJunk | flagGone},
		{"KX", flagKeep | flagGone},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.f, parseFlag([]byte(tc.s)), "%q", tc)
		assert.Equal(t, tc.s, tc.f.String(), "%q", tc)
	}

	assert.Panics(t, func() { _ = parseFlag([]byte("XX")) })
	assert.Panics(t, func() { _ = Flag(64).String() })

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