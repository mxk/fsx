package main

import (
	"github.com/mxk/go-cli"

	// CLI registration
	_ "github.com/mxk/fsx/cmd"
)

func main() {
	cli.Main.Summary = "File system toolbox"
	cli.Main.Run()
}
