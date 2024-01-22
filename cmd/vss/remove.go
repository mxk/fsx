//go:build windows

package vss

import (
	"github.com/mxk/go-cli"
	"github.com/mxk/go-vss"
)

var _ = vssCli.Add(&cli.Cfg{
	Name:    "remove|rm",
	Usage:   "<id|link>",
	Summary: "Remove a shadow copy by ID or symlink path",
	MinArgs: 1,
	MaxArgs: 1,
	New:     func() cli.Cmd { return removeCmd{} },
})

type removeCmd struct{}

func (removeCmd) Main(args []string) error {
	return vss.Remove(args[0])
}
