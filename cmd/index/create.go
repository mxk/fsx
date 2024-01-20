package index

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/mxk/go-cli"

	"github.com/mxk/fsx/index"
)

var _ = indexCli.Add(&cli.Cfg{
	Name:    "create|c",
	Usage:   "<out-db> <dir>",
	Summary: "Create a new file system index",
	MinArgs: 2,
	MaxArgs: 2,
	New:     func() cli.Cmd { return indexCreateCmd{} },
})

type indexCreateCmd struct{}

func (indexCreateCmd) Main(args []string) error {
	root := filepath.Clean(args[1])
	ctx := context.Background()
	var walkErr bool
	errFn := func(err error) {
		walkErr = true
		log.Println(err)
	}
	var lastProgReport time.Time
	idx, err := index.Build(ctx, os.DirFS(root), errFn, func(p *index.Progress) {
		if p.IsFinal() || p.Time().Sub(lastProgReport) >= 5*time.Minute {
			lastProgReport = p.Time()
			log.Println(p)
		}
	})
	if err != nil {
		return err
	}
	err = cli.WriteFileAtomic(args[0], func(f *os.File) error {
		return idx.Write(f)
	})
	if err == nil && walkErr {
		err = cli.ExitCode(1)
	}
	return err
}
