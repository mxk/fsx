package index

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testIdx = `fsx index v1
/
	d1/a
	d2/a
	a
		0100000000000000000000000000000000000000000000000000000000000000	1	2009-11-10T23:00:00Z
	b
		0200000000000000000000000000000000000000000000000000000000000000	2	2009-11-11T23:00:01Z
	c
	d
	e
	f
		0300000000000000000000000000000000000000000000000000000000000000	3	2009-11-10T23:00:00Z,,2009-11-11T23:00:01Z
`

func TestIndex(t *testing.T) {
	d1, d2, d3 := Digest{1}, Digest{2}, Digest{3}
	t0, err := time.Parse(time.RFC3339Nano, "2009-11-10T23:00:00Z")
	require.NoError(t, err)
	t1, err := time.Parse(time.RFC3339Nano, "2009-11-11T23:00:01Z")
	require.NoError(t, err)
	want := Index{
		root: "/",
		groups: []Files{{
			{Path{"d1/a"}, d1, 1, t0},
			{Path{"d2/a"}, d1, 1, t0},
			{Path{"a"}, d1, 1, t0},
		}, {
			{Path{"b"}, d2, 2, t1},
		}, {
			{Path{"c"}, d3, 3, t0},
			{Path{"d"}, d3, 3, t0},
			{Path{"e"}, d3, 3, t1},
			{Path{"f"}, d3, 3, t1},
		}},
	}

	// Without compression
	var buf bytes.Buffer
	require.NoError(t, want.WriteRaw(&buf))
	require.Equal(t, testIdx, buf.String())
	have, err := read(&buf)
	require.NoError(t, err)
	require.Equal(t, want, have)

	// With compression
	buf.Reset()
	require.NoError(t, want.Write(&buf))
	have, err = Read(&buf)
	require.NoError(t, err)
	require.Equal(t, want, have)
}
