//go:build windows

package cmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/mxk/go-cli"
	"github.com/mxk/go-vss"
)

var _ = vssCli.Add(&cli.Cfg{
	Name:    "list|ls",
	Usage:   "[-vol <name>]",
	Summary: "List existing shadow copies",
	New:     func() cli.Cmd { return &vssListCmd{} },
})

type vssListCmd struct {
	Vol string `cli:"Filter by volume {name} (e.g. 'C:')"`
}

func (cmd *vssListCmd) Main([]string) error {
	all, err := vss.List(cmd.Vol)
	if err != nil {
		return err
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].InstallDate.Before(all[j].InstallDate)
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
