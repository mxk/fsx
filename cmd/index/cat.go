package index

import (
	"os"

	"github.com/mxk/go-cli"

	"github.com/mxk/fsx/index"
)

var _ = indexCli.Add(&cli.Cfg{
	Name:    "cat",
	Usage:   "<db> ...",
	Summary: "Write file system index to stdout",
	MinArgs: 1,
	New:     func() cli.Cmd { return indexCatCmd{} },
})

type indexCatCmd struct{}

func (indexCatCmd) Main(args []string) error {
	for _, name := range args {
		idx, err := index.Load(name)
		if err != nil {
			return err
		}
		if err = idx.WriteRaw(os.Stdout); err != nil {
			return err
		}
	}
	return nil
}
