package main

import (
	_ "embed"
	"fmt"
	"github.com/woofdoggo/resetti/cmd"
	"os"
	"strings"
)

//go:embed .notice
var notice string

//go:embed .version
var version string

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "--help":
		printHelp()
	case "--version":
		fmt.Print(
			"\n    resetti ",
			strings.Trim(version, "\n"),
			" - Minecraft resetting macro\n",
			notice,
		)
	case "obs":
		cmd.CmdObs()
	default:
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`
    resetti - Minecraft resetting macro

    USAGE:
        --help                  Print this menu.
        --version               Print the version and copyright notice.

    SUBCOMMANDS:
        resetti obs             Setup OBS for resetti.
    `)
}
