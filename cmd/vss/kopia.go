//go:build windows

package vss

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mxk/go-cli"
	"github.com/mxk/go-vss"
)

var _ = vssCli.Add(&cli.Cfg{
	Name:    "kopia",
	Summary: "Manage shadow copies for Kopia backup actions",
	New: func() cli.Cmd {
		mnt := os.Getenv("SystemDrive")
		if mnt == "" {
			mnt = "C:"
		}
		return &vssKopiaCmd{mnt + `\`}
	},
})

type vssKopiaCmd struct {
	Mnt string `cli:"Root {directory} where to mount shadow copies"`
}

func (*vssKopiaCmd) Help(w *cli.Writer) {
	w.Text(`
	This command can be set as the Before/After Snapshot action in Kopia to use
	the Volume Shadow Copy Service for consistent backups. It performs the same
	operations as the example PowerShell scripts:

	https://kopia.io/docs/advanced/actions/#windows-shadow-copy

	Kopia must have admin privileges for this command to work. When using
	KopiaUI, disable its "Launch At Startup" option and create a task in the
	Windows Task Scheduler to run KopiaUI.exe at log on with the highest
	privileges of a user who is a member of the Administrators group.

	By default, shadow copies are mounted under %SystemDrive% to ensure their
	visibility in case something goes wrong. The -mnt option can be used to set
	a different root, which must already exist.
	`)
}

func (cmd *vssKopiaCmd) Main([]string) error {
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
	if cmd.Mnt, err = filepath.Abs(cmd.Mnt); err != nil {
		return err
	}
	snapRoot := filepath.Join(cmd.Mnt, fmt.Sprintf(".kopia-%s-%s",
		strings.NewReplacer(`:`, ``, `\`, ``).Replace(srcVol), snapID))
	switch action {
	case "before-snapshot-root":
		if _, err = os.Stat(snapRoot); !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("path already exists: %s", snapRoot)
		}
		err := vss.CreateLink(snapRoot, srcVol)
		if err == nil {
			_, err = fmt.Printf("KOPIA_SNAPSHOT_PATH=%s\n", filepath.Join(snapRoot, srcRel))
		}
		return err
	case "after-snapshot-root":
		if want := filepath.Join(snapRoot, srcRel); filepath.Clean(snapPath) != want {
			return fmt.Errorf("unexpected KOPIA_SNAPSHOT_PATH: %s (expecting: %s)", snapPath, want)
		}
		return vss.Remove(snapRoot)
	}
	return cli.Errorf("unsupported KOPIA_ACTION: %s", action)
}
