package cmd

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/mc"
	"github.com/woofdoggo/resetti/internal/reset"
	"github.com/woofdoggo/resetti/internal/x11"
)

func Run() {
	// Setup logger output.
	logPath, ok := os.LookupEnv("RESETTI_LOG_PATH")
	if !ok {
		logPath = "/tmp/resetti.log"
	}
	logFile, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		fmt.Println("Failed to open log:", err)
		os.Exit(1)
	}
	logWriter := io.MultiWriter(logFile, os.Stdout)
	log.SetOutput(logWriter)

	// Get configuration.
	profileName := os.Args[1]
	profile, err := cfg.GetProfile(profileName)
	if err != nil {
		fmt.Println("Failed to get profile:", err)
		os.Exit(1)
	}

	// Connect to the X server.
	x, err := x11.NewClient()
	if err != nil {
		fmt.Println("Failed to start X11 connection:", err)
		os.Exit(1)
	}

	// Find Minecraft instances.
	instanceInfo, err := mc.FindInstances(x)
	if err != nil {
		fmt.Println("Failed to find instances:", err)
		os.Exit(1)
	}

	switch profile.General.ResetType {
	case "standard":
		multi := reset.NewMulti(profile, instanceInfo, x)
		if err != nil {
			fmt.Println("Failed to start multi:", err)
			os.Exit(1)
		}
		err = multi.Run()
		if err != nil {
			fmt.Println("Multi failed:", err)
			os.Exit(1)
		}
	case "wall":
		wall := reset.NewWall(profile, instanceInfo, x)
		if err != nil {
			fmt.Println("Failed to start wall:", err)
			os.Exit(1)
		}
		err = wall.Run()
		if err != nil {
			fmt.Println("Wall failed:", err)
			os.Exit(1)
		}
	}
}
