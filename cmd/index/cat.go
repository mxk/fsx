package index

import (
	"io"
	"os"

	"github.com/klauspost/compress/zstd"
	"github.com/mxk/go-cli"
)

var _ = indexCli.Add(&cli.Cfg{
	Name:    "cat",
	Usage:   "<db>",
	Summary: "Write file system index to stdout",
	MinArgs: 1,
	MaxArgs: 1,
	New:     func() cli.Cmd { return indexCatCmd{} },
})

type indexCatCmd struct{}

func (indexCatCmd) Main(args []string) error {
	f, err := os.Open(args[0])
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	r, err := zstd.NewReader(f)
	if err == nil {
		_, err = io.Copy(os.Stdout, r)
	}
	return err
}
