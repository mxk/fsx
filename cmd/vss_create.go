//go:build windows

package cmd

import (
	"fmt"

	"github.com/mxk/go-cli"
	"github.com/mxk/go-vss"
)

var vssCreateCli = vssCli.Add(&cli.Cfg{
	Name:    "create|mk",
	Usage:   "[-link <path>] <volume>",
	Summary: "Create a shadow copy",
	MinArgs: 1,
	MaxArgs: 1,
	New:     func() cli.Cmd { return &vssCreateCmd{} },
})

type vssCreateCmd struct {
	Link string `cli:"Symlink {path} where to mount the new shadow copy"`
}

func (*vssCreateCmd) Info() *cli.Cfg { return vssCreateCli }

func (cmd *vssCreateCmd) Main(args []string) error {
	if cmd.Link != "" {
		return vss.CreateAt(cmd.Link, args[0])
	}
	id, err := vss.Create(args[0])
	if err == nil {
		_, err = fmt.Println(id)
	}
	return err
}
