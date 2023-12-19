package cmd

import (
	"fmt"

	"github.com/mxk/fsx/index"
	"github.com/mxk/go-cli"
)

var dupsCli = cli.Main.Add(&cli.Cfg{
	Name:    "dups",
	Usage:   "<db>",
	Summary: "Find duplicate directories",
	MinArgs: 1,
	MaxArgs: 1,
	New:     func() cli.Cmd { return &dupCmd{} },
})

type dupCmd struct{}

func (*dupCmd) Info() *cli.Cfg { return dupsCli }

func (cmd *dupCmd) Main(args []string) error {
	idx, err := index.Load(args[0])
	if err != nil {
		return err
	}
	t := idx.ToTree()
	for _, dup := range t.Dups() {
		fmt.Println(dup.Dir.Path)
		for _, mir := range dup.Alt {
			fmt.Printf("\t%s\n", mir.Path)
		}
	}
	return nil
}
