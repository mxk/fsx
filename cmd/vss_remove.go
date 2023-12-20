//go:build windows

package cmd

import (
	"github.com/mxk/fsx/vss"
	"github.com/mxk/go-cli"
)

var vssRemoveCli = vssCli.Add(&cli.Cfg{
	Name:    "remove|rm",
	Usage:   "<id|link>",
	Summary: "Remove a shadow copy by ID or symlink path",
	MinArgs: 1,
	MaxArgs: 1,
	New:     func() cli.Cmd { return vssRemoveCmd{} },
})

type vssRemoveCmd struct{}

func (vssRemoveCmd) Info() *cli.Cfg { return vssRemoveCli }

func (vssRemoveCmd) Main(args []string) error {
	return vss.Remove(args[0])
}
