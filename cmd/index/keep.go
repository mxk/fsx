package index

import (
	"github.com/mxk/go-cli"

	"github.com/mxk/fsx/index"
)

var _ = indexCli.Add(&cli.Cfg{
	Name:    "keep",
	Usage:   "<index> <path> ...",
	Summary: "Protect files and directories from deduplication",
	MinArgs: 2,
	New:     func() cli.Cmd { return keepCmd{} },
})

type keepCmd struct{}

func (keepCmd) Main(args []string) error {
	idx, err := index.Load(args[0])
	if err != nil {
		return err
	}
	for _, name := range args[1:] {
		if err = idx.ToTree().MarkKeep(name); err != nil {
			return err
		}
	}
	return idx.Save(args[0])
}
