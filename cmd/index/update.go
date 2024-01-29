package index

import (
	"context"
	"os"

	"github.com/mxk/go-cli"

	"github.com/mxk/fsx/index"
)

var _ = indexCli.Add(&cli.Cfg{
	Name:    "update|u",
	Usage:   "<index>",
	Summary: "Update file system index",
	MinArgs: 1,
	MaxArgs: 1,
	New:     func() cli.Cmd { return &updateCmd{} },
})

type updateCmd struct {
	Root string `cli:"Change root directory"`
}

func (cmd *updateCmd) Main(args []string) error {
	x, err := index.Load(args[0])
	if err != nil {
		return err
	}
	if cmd.Root == "" {
		cmd.Root = x.Root()
	}
	if _, err := os.Stat(cmd.Root); err != nil {
		return err
	}
	var m monitor
	x, err = x.ToTree().Rescan(context.Background(), os.DirFS(cmd.Root), m.err, m.report)
	if err != nil {
		return err
	}
	if err = x.Save(args[0]); err == nil && m.walkErr {
		err = cli.ExitCode(1)
	}
	return err
}
