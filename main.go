package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/woofdoggo/resetti/cmd"
	"github.com/woofdoggo/resetti/internal/cfg"
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
	case "--help", "-h", "help":
		printHelp()
	case "--version", "version":
		fmt.Print(
			"\n    resetti ",
			strings.Trim(version, "\n"),
			" - Minecraft resetting macro\n",
			notice,
		)
	case "new":
		if len(os.Args) < 3 {
			printHelp()
			os.Exit(1)
		}
		err := cfg.MakeProfile(os.Args[2])
		if err != nil {
			fmt.Println("Failed to make profile:", err)
		} else {
			fmt.Println("Created profile!")
		}
	default:
		cmd.Run()
	}
}

func printHelp() {
	fmt.Println(`
    resetti - Minecraft resetting macro
    USAGE:
        resetti [PROFILE]       Run resetti with the given profile.

    SUBCOMMANDS:
        resetti new [PROFILE]   Create a new profile named PROFILE with
                                the default configuration.
        resetti help            Print this message.
        resetti version         Get the version of resetti installed.
    `)
}
