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
	Usage:   "<index> <root>",
	Summary: "Create a new file system index",
	MinArgs: 2,
	MaxArgs: 2,
	New:     func() cli.Cmd { return createCmd{} },
})

type createCmd struct{}

func (createCmd) Main(args []string) error {
	root := filepath.Clean(args[1])
	var m monitor
	idx, err := index.Scan(context.Background(), os.DirFS(root), m.err, m.report)
	if err != nil {
		return err
	}
	err = cli.WriteFileAtomic(args[0], func(f *os.File) error { return idx.Write(f) })
	if err == nil && m.walkErr {
		err = cli.ExitCode(1)
	}
	return err
}

type monitor struct {
	walkErr    bool
	lastReport time.Time
}

func (m *monitor) err(err error) {
	m.walkErr = true
	log.Println(err)
}

func (m *monitor) report(p *index.Progress) {
	if p.IsFinal() || p.Time().Sub(m.lastReport) >= 5*time.Minute {
		m.lastReport = p.Time()
		log.Println(p)
	}
}
