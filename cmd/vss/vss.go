//go:build windows

package vss

import "github.com/mxk/go-cli"

var vssCli = cli.Main.Add(&cli.Cfg{
	Name:    "vss",
	Summary: "Windows shadow copy management commands",
})
