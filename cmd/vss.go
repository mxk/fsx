//go:build windows

package cmd

import "github.com/mxk/go-cli"

var vssCli = cli.Main.Add(&cli.Cfg{
	Name:    "vss",
	Summary: "Windows shadow copy management commands",
})
