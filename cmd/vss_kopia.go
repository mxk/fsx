//go:build windows

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mxk/go-cli"
	"github.com/mxk/go-vss"
)

var _ = vssCli.Add(&cli.Cfg{
	Name:    "kopia",
	Summary: "Manage shadow copies for Kopia backup actions",
	New:     func() cli.Cmd { return vssKopiaCmd{} },
})

type vssKopiaCmd struct{}

func (vssKopiaCmd) Help(w *cli.Writer) {
	w.Text(`
	This command can be set as the Before/After Snapshot action in Kopia to use
	the Volume Shadow Copy Service for consistent backups. It performs the same
	operations as the example PowerShell scripts:

	https://kopia.io/docs/advanced/actions/#windows-shadow-copy

	Kopia must have admin privileges for this command to work. When using
	KopiaUI, disable its "Launch At Startup" option and create a task in the
	Windows Task Scheduler to run KopiaUI.exe at log on with the highest
	privileges of a user who is a member of the Administrators group.
	`)
}

func (vssKopiaCmd) Main([]string) error {
	var (
		action   = os.Getenv("KOPIA_ACTION")
		snapID   = os.Getenv("KOPIA_SNAPSHOT_ID")
		srcPath  = filepath.FromSlash(os.Getenv("KOPIA_SOURCE_PATH"))
		snapPath = filepath.FromSlash(os.Getenv("KOPIA_SNAPSHOT_PATH"))
		version  = os.Getenv("KOPIA_VERSION")
	)
	if snapID == "" || srcPath == "" || snapPath == "" || version == "" {
		return cli.Error("missing one or more KOPIA_* environment variables")
	}
	srcVol, srcRel, err := vss.SplitVolume(srcPath)
	if err != nil {
		return err
	}
	switch action {
	case "before-snapshot-root":
		return create(srcVol, srcRel)
	case "after-snapshot-root":
		if dev := deviceObject(snapPath); dev != "" {
			return vss.Remove(dev)
		}
		return cli.Errorf("invalid KOPIA_SNAPSHOT_PATH: %s", snapPath)
	}
	return cli.Errorf("unsupported KOPIA_ACTION: %s", action)
}

func create(srcVol, srcRel string) (err error) {
	id, err := vss.Create(srcVol)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = vss.Remove(id)
		}
	}()
	sc, err := vss.Get(id)
	if err == nil {
		_, err = fmt.Printf("KOPIA_SNAPSHOT_PATH=%s\\\n", filepath.Join(sc.DeviceObject, srcRel))
	}
	return err
}

func deviceObject(name string) string {
	name = filepath.FromSlash(name)
	i := strings.Index(name, "HarddiskVolumeShadowCopy")
	j := strings.IndexByte(name[i+1:], filepath.Separator)
	if i < 0 || j < 0 {
		return ""
	}
	return name[:i+1+j]
}
