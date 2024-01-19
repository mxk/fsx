package index

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttr(t *testing.T) {
	tests := []struct {
		s string
		a Attr
	}{
		{"", 0},
		{"D", attrDup},
		{"J", attrJunk},
		{"K", attrKeep},
		{"X", attrGone},
		{"DX", attrDup | attrGone},
		{"JX", attrJunk | attrGone},
		{"KX", attrKeep | attrGone},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.a, parseAttr([]byte(tc.s)), "%q", tc)
		assert.Equal(t, tc.s, tc.a.String(), "%q", tc)
	}

	assert.Panics(t, func() { _ = parseAttr([]byte("XX")) })
	assert.Panics(t, func() { _ = Attr(64).String() })

	assert.True(t, attrDup.IsDup())
	assert.False(t, attrDup.IsJunk())
	assert.False(t, attrDup.Keep())
	assert.False(t, attrDup.IsGone())
	assert.True(t, attrDup.MayRemove())

	a := attrKeep | attrGone
	assert.False(t, a.IsDup())
	assert.False(t, a.IsJunk())
	assert.True(t, a.Keep())
	assert.True(t, a.IsGone())
	assert.False(t, a.MayRemove())
}
