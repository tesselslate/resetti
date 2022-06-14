package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/woofdoggo/resetti/cfg"
	"github.com/woofdoggo/resetti/cmd"
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
		conf, err := cfg.GetProfile(os.Args[1])
		if err != nil {
			fmt.Println("Failed to get profile:", err)
			os.Exit(1)
		}
		os.Exit(cmd.CmdReset(conf))
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
