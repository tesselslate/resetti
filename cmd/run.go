package cmd

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/woofdoggo/resetti/internal/cfg"
	"github.com/woofdoggo/resetti/internal/reset"
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
	if err = reset.Run(profile); err != nil {
		fmt.Println("Failed to launch:", err)
		os.Exit(1)
	}
}
