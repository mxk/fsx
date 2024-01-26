package index

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testIdx = `fsx index v1
/
K	d1/a	//											2009-11-10T23:00:00Z
	d2/a
	a
		0100000000000000000000000000000000000000000000000000000000000000	1
	b	//											2009-11-11T23:00:01Z
		0200000000000000000000000000000000000000000000000000000000000000	2
	c	//											2009-11-10T23:00:00Z
D	d		//
DX	e 		//										2009-11-11T23:00:01Z
	f
		0300000000000000000000000000000000000000000000000000000000000000	3
`

func TestIndexReadWrite(t *testing.T) {
	d1, d2, d3 := Digest{1}, Digest{2}, Digest{3}
	t0 := time.Date(2009, 11, 10, 23, 00, 00, 0, time.UTC)
	t1 := time.Date(2009, 11, 11, 23, 00, 01, 0, time.UTC)

	want := &Index{
		root: "/",
		groups: []Files{{
			{Path{"d1/a"}, d1, 1, t0, flagKeep},
			{Path{"d2/a"}, d1, 1, t0, flagNone},
			{Path{"a"}, d1, 1, t0, flagNone},
		}, {
			{Path{"b"}, d2, 2, t1, flagNone},
			{Path{"gone1"}, d2, 2, t1, flagGone},
		}, {
			{Path{"gone2"}, d2, 2, t1, flagGone},
		}, {
			{Path{"c"}, d3, 3, t0, flagNone},
			{Path{"d\t"}, d3, 3, t0, flagDup},
			{Path{"e \t"}, d3, 3, t1, flagDup | flagGone},
			{Path{"f"}, d3, 3, t1, flagNone},
		}},
	}

	// Text format without compression
	var buf bytes.Buffer
	require.NoError(t, want.write(&buf))
	require.Equal(t, testIdx, buf.String())
	have, err := read(&buf)
	require.NoError(t, err)
	want.groups[1] = want.groups[1][:1]
	want.groups = append(want.groups[:2], want.groups[3])
	require.Equal(t, want, have)

	// Roundtrip with compression
	buf.Reset()
	require.NoError(t, want.Write(&buf))
	have, err = Read(&buf)
	require.NoError(t, err)
	require.Equal(t, want, have)
}
