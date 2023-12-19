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
`

func TestIndex(t *testing.T) {
	d1 := Digest{1}
	d2 := Digest{2}
	t1, _ := time.Parse(time.RFC3339Nano, "2009-11-10T23:00:00Z")
	t2, _ := time.Parse(time.RFC3339Nano, "2009-11-11T23:00:01Z")
	want := Index{
		Root: "/",
		Groups: []Files{
			{{Path{"d1/a"}, d1, 1, t1}, {Path{"d2/a"}, d1, 1, t1}, {Path{"a"}, d1, 1, t1}},
			{{Path{"b"}, d2, 2, t2}},
		},
	}
	var buf bytes.Buffer
	require.NoError(t, want.Write(&buf))
	require.Equal(t, testIdx, buf.String())
	have, err := Read(&buf)
	require.NoError(t, err)
	require.Equal(t, want, have)
}
