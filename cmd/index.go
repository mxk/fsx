//go:build windows

package cmd

import "github.com/mxk/go-cli"

var indexCli = cli.Main.Add(&cli.Cfg{
	Name:    "index",
	Summary: "File system index commands",
})
