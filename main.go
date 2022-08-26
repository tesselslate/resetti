package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/reset"
	"github.com/woofdoggo/resetti/internal/ui"
)

//go:embed .notice
var notice string

//go:embed .version
var version string

func main() {
	if len(os.Args) < 2 {
		confName, err := ui.ShowProfileMenu()
		if err != nil {
			fmt.Println("Failed to open menu:", err)
			os.Exit(1)
		}
		if confName == "" {
			os.Exit(1)
		}
		conf, err := cfg.GetProfile(confName)
		if err != nil {
			fmt.Println("Failed to load profile:", err)
			os.Exit(1)
		}
		switch conf.General.ResetType {
		case "standard":
			err = reset.ResetCycle(*conf)
		case "wall":
			err = reset.ResetWall(*conf)
		case "setseed":
			err = reset.ResetSetseed(*conf)
		}
		if err != nil {
			fmt.Println("resetti failed:")
			fmt.Println(err)
			os.Exit(1)
		}
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
	case "obs":
		ui.ShowObsSetup()
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
		conf, err := cfg.GetProfile(os.Args[1])
		if err != nil {
			fmt.Println("Failed to load profile:", err)
			os.Exit(1)
		}
		switch conf.General.ResetType {
		case "standard":
			err = reset.ResetCycle(*conf)
		case "wall":
			err = reset.ResetWall(*conf)
		case "setseed":
			err = reset.ResetSetseed(*conf)
		}
		if err != nil {
			fmt.Println("resetti failed:")
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

func printHelp() {
	fmt.Println(`
    resetti - Minecraft resetting macro
    USAGE:
        resetti [PROFILE]       Run resetti with the given profile.
                                If no profile is provided, the menu
                                is opened.
    SUBCOMMANDS:
        resetti new [PROFILE]   Create a new profile named PROFILE with
                                the default configuration.
        resetti help            Print this message.
        resetti version         Get the version of resetti installed.
        resetti obs [ARGUMENTS] Setup OBS for resetti.
          --port=PORT
          --pass=PASSWORD
          --lockImg=PATH
    `)
}
