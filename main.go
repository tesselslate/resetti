package main

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/ctl"
	"github.com/woofdoggo/resetti/internal/log"
	"github.com/woofdoggo/resetti/internal/res"
)

//go:embed .notice
var notice string

//go:embed .version
var version string

func main() {
	// Setup logger output.
	logPath, ok := os.LookupEnv("RESETTI_LOG_PATH")
	if !ok {
		logPath = "/tmp/resetti.log"
	}

	// TODO: Add log statements throughout.
	logger := log.NewLogger(log.INFO, logPath, log.DefaultFormatter())
	logger.Info("Started Logger")

	if err := res.WriteResources(); err != nil {
		fmt.Println("Failed to write resources:", err)
		os.Exit(1)
	}
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
		Run()
	}
}

func Run() {
	// Get configuration and run.
	profileName := os.Args[1]
	profile, err := cfg.GetProfile(profileName)
	if err != nil {
		fmt.Println("Failed to get profile:", err)
		os.Exit(1)
	}
	if err = ctl.Run(&profile); err != nil {
		fmt.Println("Failed to run:", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`
    resetti - Minecraft resetting macro
    USAGE:
        resetti [PROFILE]       Run resetti with the given profile.
          --force-cgroups       Force the cgroup setup script to run.
          --force-log           Force the latest.log reader to be used.
          --force-wpstate       Force the wpstateout.txt reader to be used.

    SUBCOMMANDS:
        resetti new [PROFILE]   Create a new profile named PROFILE with
                                the default configuration.
        resetti help            Print this message.
        resetti version         Get the version of resetti installed.
    `)
}
