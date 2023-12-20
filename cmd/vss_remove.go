//go:build windows

package cmd

import (
	"github.com/mxk/fsx/vss"
	"github.com/mxk/go-cli"
)

var vssDeleteCli = vssCli.Add(&cli.Cfg{
	Name:    "delete|rm",
	Usage:   "<id|link>",
	Summary: "Delete a shadow copy by ID or symlink path",
	MinArgs: 1,
	MaxArgs: 1,
	New:     func() cli.Cmd { return vssDeleteCmd{} },
})

type vssDeleteCmd struct{}

func (vssDeleteCmd) Info() *cli.Cfg { return vssDeleteCli }

func (vssDeleteCmd) Main(args []string) error {
	return vss.Delete(args[0])
}
