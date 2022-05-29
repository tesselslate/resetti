package main

import (
	_ "embed"
	"fmt"
	"os"
	"resetti/cfg"
	"resetti/cmd/reset"
	"resetti/cmd/setup"
	"strings"

	"gopkg.in/yaml.v2"
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
	case "--save-default":
		confBytes, err := yaml.Marshal(cfg.DefaultConfig)
		if err != nil {
			fmt.Println("Failed to serialize config:", err)
			return
		}
		confPath, err := cfg.GetPath()
		if err != nil {
			fmt.Println("Failed to get config path:", err)
			return
		}
		err = os.WriteFile(confPath, confBytes, 0644)
		if err != nil {
			fmt.Println("Failed to write config:", err)
			return
		}
		fmt.Printf("Wrote default configuration to:\n%s\n", confPath)
		return
	case "--version":
		fmt.Print(
			"\n    resetti ",
			strings.Trim(version, "\n"),
			" - Minecraft resetting macro\n",
			notice,
		)
	case "cycle":
		reset.CmdCycle()
	case "wall":
		reset.CmdWall()
	case "keys":
		setup.CmdKeys()
	case "obs":
		setup.CmdObs()
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
        --save-default          Save default config.
        --version               Print the version and copyright notice.

    SUBCOMMANDS:
        resetti cycle           Run the "standard" resetter. Cycles
                                between insatnces sequentially.
                                Supports both single- and multi-instance.

        resetti wall            Run the "wall" style resetter.
                                Requires OBS.

        resetti keys            Setup keybinds for resetti.

        resetti obs             Setup OBS for resetti.
    `)
}
