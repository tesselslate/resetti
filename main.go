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

	logger := log.DefaultLogger("resetti", log.INFO, logPath)
	logger.Info("Started Logger")
	defer func() {
		logger.Close()
	}()

	if err := res.WriteResources(); err != nil {
		logger.Error("Failed to write resources: %s", err)
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
			logger.Error("Failed to make profile: %s", err)
		} else {
			logger.Info("Created profile!")
		}
	case "-d", "--debug":
		logger.Info("Running in debug mode.")
		logger.SetLevel(log.DEBUG)
		if len(os.Args) < 3 {
			logger.Error("Expected profile name after -d, --debug.")
			printHelp()
			os.Exit(1)
		}
		profileName := os.Args[2]
		Run(profileName)
	default:
		if len(os.Args) >= 3 {
			if os.Args[2] == "-d" || os.Args[2] == "--debug" {
				logger.Info("Running in debug mode.")
				logger.SetLevel(log.DEBUG)
			}
		}
		profileName := os.Args[1]
		Run(profileName)
	}
}

func Run(profileName string) {
	// Get configuration and run.
	logger := log.FromName("resetti")
	profile, err := cfg.GetProfile(profileName)
	if err != nil {
		logger.Error("Failed to get profile: %s", err)
		os.Exit(1)
	}
	if err = ctl.Run(&profile); err != nil {
		logger.Error("Failed to run: %s", err)
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
          -d, --debug           Run resetti in debug mode.

    SUBCOMMANDS:
        resetti new [PROFILE]   Create a new profile named PROFILE with
                                the default configuration.
        resetti help            Print this message.
        resetti version         Get the version of resetti installed.
    `)
}
