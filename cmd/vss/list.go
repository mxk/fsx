//go:build windows

package vss

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/mxk/go-cli"
	"github.com/mxk/go-vss"
)

var _ = vssCli.Add(&cli.Cfg{
	Name:    "list|ls",
	Usage:   "[-vol <name>]",
	Summary: "List existing shadow copies",
	New:     func() cli.Cmd { return &listCmd{} },
})

type listCmd struct {
	Vol string `cli:"Filter by volume {name} (e.g. 'C:')"`
}

func (cmd *listCmd) Main([]string) error {
	all, err := vss.List(cmd.Vol)
	if err != nil {
		return err
	}
	slices.SortFunc(all, func(a, b *vss.ShadowCopy) int {
		return a.InstallDate.Compare(b.InstallDate)
	})
	w := bufio.NewWriter(os.Stdout)
	for _, sc := range all {
		path, err := sc.VolumePath()
		if path == "" {
			path = sc.VolumeName
		}
		_, _ = w.WriteString(path)
		if _ = w.WriteByte(' '); err != nil {
			fmt.Fprintf(w, "(%s) ", err)
		}
		fmt.Fprintln(w, sc.InstallDate.Format(time.DateTime), sc.ID, sc.DeviceObject)
	}
	return w.Flush()
}
