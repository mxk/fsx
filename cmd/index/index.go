//go:build windows

package index

import "github.com/mxk/go-cli"

var indexCli = cli.Main.Add(&cli.Cfg{
	Name:    "index",
	Summary: "File system index commands",
})
