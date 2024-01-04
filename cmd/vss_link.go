//go:build windows

package cmd

import (
	"github.com/mxk/go-cli"
	"github.com/mxk/go-vss"
)

var _ = vssCli.Add(&cli.Cfg{
	Name:    "link|ln",
	Usage:   "<id|device> <link>",
	Summary: "Link a shadow copy",
	MinArgs: 2,
	MaxArgs: 2,
	New:     func() cli.Cmd { return vssLinkCmd{} },
})

type vssLinkCmd struct{}

func (vssLinkCmd) Main(args []string) error {
	sc, err := vss.Get(args[0])
	if err == nil {
		err = sc.Link(args[1])
	}
	return err
}
